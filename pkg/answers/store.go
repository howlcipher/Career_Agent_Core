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
// storeApprovedAnswer persists the common parts of Save and SaveAbsence.
// The caller has already validated inputs and chosen kind, text, and provenance.
func (s *Store) storeApprovedAnswer(key, prompt, text string, kind Kind, sensitivity Sensitivity, scope string, reuseAllowed bool, provenance Provenance, label string, now time.Time) (Answer, error) {
	tx, err := s.conn.Begin()
	if err != nil {
		return Answer{}, fmt.Errorf("begin approved %s: %w", label, err)
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
		key, prompt, text, string(kind), string(sensitivity), scope,
		boolToInt(reuseAllowed), string(provenance), now, now); err != nil {
		return Answer{}, fmt.Errorf("store approved %s: %w", label, err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM approved_answers WHERE question_key = ? AND scope = ?`, key, scope).Scan(&id); err != nil {
		return Answer{}, fmt.Errorf("read stored approved %s: %w", label, err)
	}

	// The alias is what makes the learning loop deterministic. The exact
	// phrasing the employer used is bound to this answer, so the next time any
	// board asks it in those words the resolution is a lookup, not a match.
	if reuseAllowed {
		if _, err := tx.Exec(`INSERT INTO answer_aliases (alias_key, scope, answer_id, source, created_at)
			VALUES (?, ?, ?, 'operator', ?)
			ON CONFLICT(alias_key, scope) DO UPDATE SET answer_id = excluded.answer_id, created_at = excluded.created_at`,
			Normalize(prompt), scope, id, now); err != nil {
			return Answer{}, fmt.Errorf("store %s alias: %w", label, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, err
	}
	return Answer{
		ID: id, QuestionKey: key, CanonicalQuestion: prompt, AnswerText: text,
		Kind: kind, Sensitivity: sensitivity, Scope: scope,
		ReuseAllowed: reuseAllowed, Provenance: provenance,
		ApprovedAt: now, UpdatedAt: now,
	}, nil
}

// prepareStoreInputs validates the store connection, trims the prompt and
// value, ensures the question has a key, and chooses the scope. It is shared by
// Save and SaveAbsence; the emptyValueErr parameter lets the two callers keep
// their distinct error messages.
func (s *Store) prepareStoreInputs(promptIn, value, scopeIn, emptyValueErr string) (prompt, trimmedValue, scope, key string, now time.Time, err error) {
	if s == nil || s.conn == nil {
		return "", "", "", "", time.Time{}, errors.New("answer vault is not initialized")
	}
	prompt = strings.TrimSpace(promptIn)
	trimmedValue = strings.TrimSpace(value)
	if prompt == "" {
		return "", "", "", "", time.Time{}, errors.New("an approved answer needs the question it answers")
	}
	if trimmedValue == "" {
		return "", "", "", "", time.Time{}, errors.New(emptyValueErr)
	}
	scope = strings.TrimSpace(scopeIn)
	if scope == "" {
		scope = ScopeGlobal
	}
	key = QuestionKey(prompt)
	if key == "" {
		return "", "", "", "", time.Time{}, errors.New("the question has no recognizable text")
	}
	return prompt, trimmedValue, scope, key, s.now(), nil
}

// Save stores an approved answer, and is where the vault's central safety rule
// lives.
//
// The rule is enforced here rather than in the dashboard handler on purpose. A
// handler check protects one caller; a store check protects every caller,
// including cmd/assist, a future migration, and whatever calls this next year.
// Nothing in this package offers a way around it.
func (s *Store) Save(request SaveRequest) (Answer, error) {
	prompt, answerText, scope, key, now, err := s.prepareStoreInputs(request.Question.Prompt, request.Answer, request.Scope, "an approved answer needs an answer")
	if err != nil {
		return Answer{}, err
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

	kind := request.Kind
	if kind == "" {
		kind = KindText
	}

	return s.storeApprovedAnswer(key, prompt, answerText, kind, sensitivity, scope, request.ReuseAllowed, request.Provenance, "answer", now)
}

// SaveAbsenceRequest is the operator declaring that they do not have the thing
// a question asks for. It is a real answer: "I have no Twitter account" is a
// durable fact that resolves every optional Twitter question permanently.
type SaveAbsenceRequest struct {
	Question Question
	// Reason is a human-readable note stored as answer_text, e.g.
	// "No Twitter/X account". It must be non-empty so the vault management
	// view has something meaningful to display and the existing non-empty
	// answer_text invariant is preserved.
	Reason       string
	Scope        string
	ReuseAllowed bool
	// ReuseDecisionMade records that the operator was actually asked. Same
	// semantics as SaveRequest.ReuseDecisionMade.
	ReuseDecisionMade bool
}

// SaveAbsence stores an intentional-absence decision.
//
// It shares Save's safety rules for sensitive questions, and adds its own:
// - GeneratePerJob questions cannot be absent (they need a per-employer answer).
// - The reason must not be empty (the UI shows it, and a blank row looks broken).
//
// The approval is stored as answer_kind = 'absence' so the resolver can
// distinguish it from a value answer. The answer_text holds the reason,
// preserving the existing non-NULL TEXT invariant. Resolution-time enforcement
// of the optional-field-only rule happens in Resolve, not here: the same
// Twitter absence correctly resolves an optional field while refusing a
// required one, and that distinction is a property of the live form, not of
// the stored answer.
func (s *Store) SaveAbsence(request SaveAbsenceRequest) (Answer, error) {
	prompt, reason, scope, key, now, err := s.prepareStoreInputs(request.Question.Prompt, request.Reason, request.Scope, "an intentional absence needs a reason (e.g. \"No Twitter/X account\")")
	if err != nil {
		return Answer{}, err
	}

	classified := Classify(request.Question)
	if classified == GeneratePerJob {
		return Answer{}, ErrNotReusable
	}
	if classified == Sensitive {
		if !request.ReuseDecisionMade || !request.ReuseAllowed {
			return Answer{}, ErrSensitiveNeedsApproval
		}
	}

	return s.storeApprovedAnswer(key, prompt, reason, KindAbsence, classified, scope, request.ReuseAllowed, OperatorApproved, "absence", now)
}

// ErrSensitiveAliasNeedsConfirmation is returned when a caller tries to bind
// extra phrasings to a sensitive answer without the operator having confirmed
// that those phrasings mean the same thing.
//
// It is separate from ErrSensitiveNeedsApproval because it is a separate
// decision. Approving an attestation answers one question the operator read.
// Aliasing it says that four other wordings ask that same question -- and the
// vocabulary these questions are drawn from is small and treacherous
// (authorization and sponsorship share most of their tokens and mean opposite
// things). Career Agent may propose the grouping; only the operator may accept
// it.
var ErrSensitiveAliasNeedsConfirmation = errors.New("binding extra phrasings to a declaration needs the operator to confirm they mean the same thing")

// AddAliases binds additional phrasings of a question to an answer already in
// the vault, so a wording the operator has not seen before still resolves to
// what they approved.
//
// Aliases are only meaningful on an answer whose reuse the operator granted: an
// answer held back as a suggestion is not something Career Agent types, so
// recognising more ways of asking for it would change nothing. Adding one to
// such an answer is refused rather than ignored, because silently doing nothing
// is how a caller comes to believe the vault knows something it does not.
func (s *Store) AddAliases(answerID int64, prompts []string, operatorConfirmedEquivalent bool) (int, error) {
	if s == nil || s.conn == nil {
		return 0, errors.New("answer vault is not initialized")
	}
	if len(prompts) == 0 {
		return 0, nil
	}
	var scope string
	var sensitivity string
	var reuseAllowed int
	if err := s.conn.QueryRow(`SELECT scope, sensitivity, reuse_allowed FROM approved_answers
		WHERE id = ? AND revoked_at IS NULL`, answerID).Scan(&scope, &sensitivity, &reuseAllowed); err != nil {
		return 0, errors.New("no live approved answer with that identifier")
	}
	if reuseAllowed == 0 {
		return 0, errors.New("an answer whose reuse is withheld cannot gain aliases")
	}
	if Sensitivity(sensitivity) == Sensitive && !operatorConfirmedEquivalent {
		return 0, ErrSensitiveAliasNeedsConfirmation
	}

	tx, err := s.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	now := s.now()
	for _, prompt := range prompts {
		alias := Normalize(prompt)
		if alias == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO answer_aliases (alias_key, scope, answer_id, source, created_at)
			VALUES (?, ?, ?, 'operator', ?)
			ON CONFLICT(alias_key, scope) DO UPDATE SET answer_id = excluded.answer_id, created_at = excluded.created_at`,
			alias, scope, answerID, now); err != nil {
			return 0, fmt.Errorf("store answer alias: %w", err)
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}

// Aliases lists the phrasings bound to one answer, so the management view can
// show the operator what Career Agent believes it recognises.
func (s *Store) Aliases(answerID int64) ([]string, error) {
	if s == nil || s.conn == nil {
		return nil, errors.New("answer vault is not initialized")
	}
	rows, err := s.conn.Query(`SELECT alias_key FROM answer_aliases WHERE answer_id = ? ORDER BY alias_key`, answerID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("list answer aliases: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		out = append(out, alias)
	}
	return out, rows.Err()
}

// Get returns one live answer by identifier.
func (s *Store) Get(answerID int64) (Answer, error) {
	if s == nil || s.conn == nil {
		return Answer{}, errors.New("answer vault is not initialized")
	}
	row := s.conn.QueryRow(`SELECT id, question_key, canonical_question, answer_text, answer_kind,
		sensitivity, scope, reuse_allowed, provenance, approved_at, updated_at, revoked_at, use_count, last_used_at
		FROM approved_answers WHERE id = ? AND revoked_at IS NULL`, answerID)
	answer, err := scanAnswer(row)
	if err != nil {
		return Answer{}, errors.New("no live approved answer with that identifier")
	}
	return answer, nil
}

// UpdateAnswer changes the text of an answer already in the vault, and may
// withdraw its reuse permission.
//
// It deliberately cannot change a question's sensitivity, its key or its scope.
// Those decide which rules apply to the row, and a path that could edit them
// would be a path around Save's refusal -- which is the one thing in this
// package nothing is allowed to route around. Changing any of them means
// revoking this answer and approving a new one, which is also the honest
// description of what actually happened.
//
// Withdrawing reuse drops the aliases with it: an alias exists to make an
// answer auto-fill, and an answer that no longer auto-fills has no use for one.
func (s *Store) UpdateAnswer(answerID int64, answerText string, reuseAllowed bool) (Answer, error) {
	if s == nil || s.conn == nil {
		return Answer{}, errors.New("answer vault is not initialized")
	}
	answerText = strings.TrimSpace(answerText)
	if answerText == "" {
		return Answer{}, errors.New("an approved answer needs an answer")
	}
	existing, err := s.Get(answerID)
	if err != nil {
		return Answer{}, err
	}
	// A declaration that has lost its reuse grant may be re-granted only through
	// the same two-decision path Save enforces, never by an edit.
	if existing.Sensitivity == Sensitive && reuseAllowed && !existing.ReuseAllowed {
		return Answer{}, ErrSensitiveNeedsApproval
	}

	tx, err := s.conn.Begin()
	if err != nil {
		return Answer{}, err
	}
	defer tx.Rollback()
	now := s.now()
	if _, err := tx.Exec(`UPDATE approved_answers
		SET answer_text = ?, reuse_allowed = ?, provenance = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL`,
		answerText, boolToInt(reuseAllowed), string(OperatorEdited), now, answerID); err != nil {
		return Answer{}, fmt.Errorf("update approved answer: %w", err)
	}
	if !reuseAllowed {
		if _, err := tx.Exec(`DELETE FROM answer_aliases WHERE answer_id = ?`, answerID); err != nil {
			return Answer{}, fmt.Errorf("remove answer aliases: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, err
	}
	existing.AnswerText = answerText
	existing.ReuseAllowed = reuseAllowed
	existing.Provenance = OperatorEdited
	existing.UpdatedAt = now
	return existing, nil
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
