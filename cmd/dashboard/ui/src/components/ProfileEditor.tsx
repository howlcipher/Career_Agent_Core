import { useEffect, useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { ProfileSnapshot } from '../types';

interface ProfileEditorProps {
  profile: ProfileSnapshot | null;
  saving: boolean;
  lastResult: string | null;
  onSave: (fields: Record<string, string>) => void;
}

/**
 * The operator's own details, edited where they live.
 *
 * This edits pii.yaml rather than keeping a second copy of it. That is the
 * whole design constraint: contact details, work status and links are already
 * the source of truth for the vault's curated patterns, and a dashboard-side
 * duplicate would be a second answer to "what is your phone number?" that could
 * silently disagree with the one the fill path uses.
 *
 * Values are masked by default where someone might be reading over the
 * operator's shoulder. That is a display choice only — everything here is
 * equally private on disk, and the server never logs or echoes any of it.
 */
export function ProfileEditor({ profile, saving, lastResult, onSave }: ProfileEditorProps) {
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  // Re-seed when the server's copy changes, but never clobber an edit in
  // progress: a 2-second poll landing mid-sentence would be maddening.
  useEffect(() => {
    if (!profile) return;
    setDrafts((current) =>
      Object.fromEntries(
        profile.fields.map((field) => [field.key, current[field.key] ?? field.value])
      )
    );
  }, [profile]);

  if (!profile) {
    return <p className="detail-meta">Loading your details…</p>;
  }

  const dirty = profile.fields.filter((field) => (drafts[field.key] ?? '') !== field.value);
  const missing = profile.fields.filter((field) => (drafts[field.key] ?? '').trim() === '');

  return (
    <div className="profile-editor">
      <p className="detail-meta">
        These are the facts Career Agent fills in for you. They are stored in{' '}
        <code>{profile.path}</code>, which stays readable only by you. Saving rewrites that file and
        keeps a timestamped backup; comments in it are not preserved.
      </p>
      {missing.length > 0 && (
        <p className="detail-meta" role="status">
          {missing.length} {missing.length === 1 ? 'detail is' : 'details are'} not set. Anything
          blank becomes a question an employer asks you instead.
        </p>
      )}
      {lastResult && (
        <p className="knowledge-result" role="status">
          {lastResult}
        </p>
      )}

      {profile.sections.map((section) => {
        const fields = profile.fields.filter((field) => field.section === section);
        if (fields.length === 0) return null;
        return (
          <fieldset key={section} className="profile-section">
            <legend>{section}</legend>
            {fields.map((field) => {
              const id = `profile-${field.key}`;
              const hidden = field.sensitive && !revealed[field.key];
              return (
                <div key={field.key} className="profile-field">
                  <label htmlFor={id}>{field.label}</label>
                  <div className="profile-field-input">
                    <input
                      id={id}
                      type={hidden ? 'password' : 'text'}
                      value={drafts[field.key] ?? ''}
                      onChange={(event) =>
                        setDrafts((current) => ({ ...current, [field.key]: event.target.value }))
                      }
                    />
                    {field.sensitive && (
                      <ConsoleButton
                        variant="ghost"
                        onClick={() =>
                          setRevealed((current) => ({ ...current, [field.key]: !current[field.key] }))
                        }
                      >
                        {hidden ? 'Show' : 'Hide'}
                      </ConsoleButton>
                    )}
                  </div>
                  {field.note && <p className="detail-meta">{field.note}</p>}
                </div>
              );
            })}
          </fieldset>
        );
      })}

      <ConsoleButton
        variant="primary"
        onClick={() =>
          onSave(Object.fromEntries(dirty.map((field) => [field.key, drafts[field.key] ?? ''])))
        }
        disabled={saving || dirty.length === 0}
      >
        {saving
          ? 'Saving…'
          : dirty.length === 0
            ? 'No changes'
            : `Save ${dirty.length} ${dirty.length === 1 ? 'change' : 'changes'}`}
      </ConsoleButton>
    </div>
  );
}
