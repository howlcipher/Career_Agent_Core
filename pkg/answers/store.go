package answers

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EnsureSchema creates the vault's tables. It follows the same shape as
// storage.EnsureAssistedSchema: idempotent CREATE IF NOT EXISTS plus additive
// ALTERs, never a destructive migration, so an existing database keeps every
// approved answer across upgrades.
func EnsureSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("answer vault schema connection is nil")
	}
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS approved_answers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		question_key TEXT NOT NULL,
		canonical_question TEXT NOT NULL,
		answer_text TEXT NOT NULL,
		answer_kind TEXT NOT NULL DEFAULT 'text',
		sensitivity TEXT NOT NULL DEFAULT 'routine',
		scope TEXT NOT NULL DEFAULT 'global',
		reuse_allowed INTEGER NOT NULL DEFAULT 0,
		provenance TEXT NOT NULL,
		approved_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		revoked_at DATETIME,
		use_count INTEGER NOT NULL DEFAULT 0,
		last_used_at DATETIME,
		UNIQUE(question_key, scope)
	);
	CREATE TABLE IF NOT EXISTS answer_aliases (
		alias_key TEXT NOT NULL,
		scope TEXT NOT NULL DEFAULT 'global',
		answer_id INTEGER NOT NULL,
		source TEXT NOT NULL DEFAULT 'operator',
		created_at DATETIME NOT NULL,
		PRIMARY KEY (alias_key, scope),
		FOREIGN KEY(answer_id) REFERENCES approved_answers(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_approved_answers_key ON approved_answers(question_key, revoked_at);
	CREATE INDEX IF NOT EXISTS idx_answer_aliases_answer ON answer_aliases(answer_id);`)
	if err != nil {
		return fmt.Errorf("create answer vault schema: %w", err)
	}
	return nil
}

// Store is the vault's persistence. It holds a *sql.DB rather than reaching
// for a package singleton so the dashboard, cmd/assist and tests can each
// supply their own connection — the same reason storage.GetAssistedQueue takes
// one explicitly.
type Store struct {
	conn *sql.DB
	// now is indirected for tests. Provenance and approval timestamps are the
	// audit trail for sensitive answers, so they have to be assertable.
	now func() time.Time
}

func NewStore(conn *sql.DB) *Store {
	return &Store{conn: conn, now: func() time.Time { return time.Now().UTC() }}
}

// SaveRequest is one approval. Every field the safety rule depends on is
// explicit and required rather than defaulted, so a caller cannot reach a
// permissive outcome by omission.
type SaveRequest struct {
	Question     Question
	Answer       string
	Kind         Kind
	Scope        string
	Provenance   Provenance
	ReuseAllowed bool
	// ReuseDecisionMade records that the operator was actually asked whether
	// this answer may be reused. It is separate from ReuseAllowed because
	// "false" and "never asked" are different states, and only the first is an
	// operator decision.
	ReuseDecisionMade bool
	// Sensitivity may be left empty to have the store classify the question
	// itself. A caller may not *lower* the classification: see Save.
	Sensitivity Sensitivity
}

// Save stores an approved answer, and is where the vault's central safety rule
// lives.
//
// The rule is enforced here rather than in the dashboard handler on purpose. A
// handler check protects one caller; a store check protects every caller,
// including cmd/assist, a future migration, and whatever calls this next year.
// Nothing in this package offers a way around it.
func (s *Store) Save(request SaveRequest) (Answer, error) {
	if s == nil || s.conn == nil {
		return Answer{}, errors.New("answer vault is not initialized")
	}
	prompt := strings.TrimSpace(request.Question.Prompt)
	answerText := strings.TrimSpace(request.Answer)
	if prompt == "" {
		return Answer{}, errors.New("an approved answer needs the question it answers")
	}
	if answerText == "" {
		return Answer{}, errors.New("an approved answer needs an answer")
	}

	// The classifier decides sensitivity from the question. A caller may raise
	// the classification but never lower it, so a client that sends
	// "routine" for an attestation cannot talk the store into treating it
	// that way.
	//
	// The caller is expected to have classified the same question the same way
	// when it decided which acknowledgements to ask the operator for. Any
	// disagreement is resolved here in favour of the stricter answer, and
	// answers.Resolve now escalates identically so the two cannot drift
	// (bugs.md #541).
	classified := Classify(request.Question)
	sensitivity := classified
	if request.Sensitivity == Sensitive {
		sensitivity = Sensitive
	}

	if sensitivity == GeneratePerJob {
		return Answer{}, ErrNotReusable
	}
	if sensitivity == Sensitive {
		operatorApproved := request.Provenance == OperatorApproved || request.Provenance == OperatorEdited
		if !operatorApproved || !request.ReuseDecisionMade || !request.ReuseAllowed {
			return Answer{}, ErrSensitiveNeedsApproval
		}
	}
	if request.Provenance == "" {
		return Answer{}, errors.New("an approved answer needs a provenance")
	}

	scope := strings.TrimSpace(request.Scope)
	if scope == "" {
		scope = ScopeGlobal
	}
	kind := request.Kind
	if kind == "" {
		kind = KindText
	}
	key := QuestionKey(prompt)
	if key == "" {
		return Answer{}, errors.New("the question has no recognizable text")
	}
	now := s.now()

	tx, err := s.conn.Begin()
	if err != nil {
		return Answer{}, fmt.Errorf("begin approved answer: %w", err)
	}
	defer tx.Rollback()

	// An approval for a question already in the vault updates that row rather
	// than adding a second one, and clears any prior revocation: re-approving
	// something is a deliberate act.
	if _, err := tx.Exec(`INSERT INTO approved_answers
		(question_key, canonical_question, answer_text, answer_kind, sensitivity, scope, reuse_allowed, provenance, approved_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(question_key, scope) DO UPDATE SET
			canonical_question = excluded.canonical_question,
			answer_text = excluded.answer_text,
			answer_kind = excluded.answer_kind,
			sensitivity = excluded.sensitivity,
			reuse_allowed = excluded.reuse_allowed,
			provenance = excluded.provenance,
			updated_at = excluded.updated_at,
			revoked_at = NULL`,
		key, prompt, answerText, string(kind), string(sensitivity), scope,
		boolToInt(request.ReuseAllowed), string(request.Provenance), now, now); err != nil {
		return Answer{}, fmt.Errorf("store approved answer: %w", err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM approved_answers WHERE question_key = ? AND scope = ?`, key, scope).Scan(&id); err != nil {
		return Answer{}, fmt.Errorf("read stored approved answer: %w", err)
	}

	// The alias is what makes the learning loop deterministic. The exact
	// phrasing the employer used is bound to this answer, so the next time any
	// board asks it in those words the resolution is a lookup, not a match.
	if request.ReuseAllowed {
		if _, err := tx.Exec(`INSERT INTO answer_aliases (alias_key, scope, answer_id, source, created_at)
			VALUES (?, ?, ?, 'operator', ?)
			ON CONFLICT(alias_key, scope) DO UPDATE SET answer_id = excluded.answer_id, created_at = excluded.created_at`,
			Normalize(prompt), scope, id, now); err != nil {
			return Answer{}, fmt.Errorf("store answer alias: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, err
	}
	return Answer{
		ID: id, QuestionKey: key, CanonicalQuestion: prompt, AnswerText: answerText,
		Kind: kind, Sensitivity: sensitivity, Scope: scope,
		ReuseAllowed: request.ReuseAllowed, Provenance: request.Provenance,
		ApprovedAt: now, UpdatedAt: now,
	}, nil
}

// Revoke withdraws an answer without deleting it. The row stays so its
// provenance remains auditable; its aliases go, because an alias exists only to
// make a live answer resolvable.
func (s *Store) Revoke(id int64) error {
	if s == nil || s.conn == nil {
		return errors.New("answer vault is not initialized")
	}
	tx, err := s.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE approved_answers SET revoked_at = ?, updated_at = ?, reuse_allowed = 0 WHERE id = ? AND revoked_at IS NULL`, s.now(), s.now(), id)
	if err != nil {
		return fmt.Errorf("revoke approved answer: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("no live approved answer with that identifier")
	}
	if _, err := tx.Exec(`DELETE FROM answer_aliases WHERE answer_id = ?`, id); err != nil {
		return fmt.Errorf("remove answer aliases: %w", err)
	}
	return tx.Commit()
}

// List returns every live answer, most recently approved first. Used by the
// dashboard's vault management view and by the operator documentation's
// "what does Career Agent know" question.
func (s *Store) List() ([]Answer, error) {
	if s == nil || s.conn == nil {
		return nil, errors.New("answer vault is not initialized")
	}
	rows, err := s.conn.Query(`SELECT id, question_key, canonical_question, answer_text, answer_kind,
		sensitivity, scope, reuse_allowed, provenance, approved_at, updated_at, revoked_at, use_count, last_used_at
		FROM approved_answers WHERE revoked_at IS NULL ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []Answer{}, nil
		}
		return nil, fmt.Errorf("list approved answers: %w", err)
	}
	defer rows.Close()
	var out []Answer
	for rows.Next() {
		answer, err := scanAnswer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, answer)
	}
	return out, rows.Err()
}

// scanRow is the shared shape of *sql.Row and *sql.Rows for scanAnswer.
type scanRow interface{ Scan(dest ...any) error }

func scanAnswer(row scanRow) (Answer, error) {
	var answer Answer
	var kind, sensitivity, provenance string
	var reuse int
	var revokedAt, lastUsedAt sql.NullTime
	if err := row.Scan(&answer.ID, &answer.QuestionKey, &answer.CanonicalQuestion, &answer.AnswerText,
		&kind, &sensitivity, &answer.Scope, &reuse, &provenance,
		&answer.ApprovedAt, &answer.UpdatedAt, &revokedAt, &answer.UseCount, &lastUsedAt); err != nil {
		return Answer{}, err
	}
	answer.Kind = Kind(kind)
	answer.Sensitivity = Sensitivity(sensitivity)
	answer.Provenance = Provenance(provenance)
	answer.ReuseAllowed = reuse != 0
	if revokedAt.Valid {
		t := revokedAt.Time
		answer.RevokedAt = &t
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		answer.LastUsedAt = &t
	}
	return answer, nil
}

// recordUse bumps the reuse counters. Failure is not propagated to the caller:
// a statistics update must never be the reason an application stalls.
func (s *Store) recordUse(id int64) {
	if s == nil || s.conn == nil || id == 0 {
		return
	}
	_, _ = s.conn.Exec(`UPDATE approved_answers SET use_count = use_count + 1, last_used_at = ? WHERE id = ?`, s.now(), id)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
