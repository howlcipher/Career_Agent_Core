import { useState } from 'react';
import type { DogfoodFeedbackCategory } from '../types';
import { ConfirmDialog } from './ConfirmDialog';
import { ConsoleButton } from './ConsoleButton';

interface DogfoodFeedbackDialogProps {
  ordinal: number;
  targetCount: number;
  subject: string;
  onSubmit: (category: DogfoodFeedbackCategory, manualCount?: number, note?: string) => Promise<boolean>;
  onSkip: () => void;
}

const OPTIONS: { value: DogfoodFeedbackCategory; label: string }[] = [
  { value: 'nothing', label: 'Nothing meaningful' },
  { value: 'bad_match', label: 'Bad/irrelevant job match' },
  { value: 'known_not_filled', label: 'Known information was not filled' },
  { value: 'filled_incorrect', label: 'Career Agent filled something incorrectly' },
  { value: 'repeated_question', label: 'Repeated question Career Agent should know' },
  { value: 'one_off_question', label: 'One-off employer question' },
  { value: 'blocker', label: 'Browser/application blocker' },
  { value: 'other', label: 'Other' },
];

/**
 * The one thing the system cannot know about a cohort application, asked in
 * a few seconds. It never asks for actual answers or PII, and it is always
 * skippable -- feedback is a nice-to-have signal, never a gate on the run.
 */
export function DogfoodFeedbackDialog({ ordinal, targetCount, subject, onSubmit, onSkip }: DogfoodFeedbackDialogProps) {
  const [category, setCategory] = useState<DogfoodFeedbackCategory | ''>('');
  const [manualCount, setManualCount] = useState('');
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async () => {
    if (!category) return;
    setSubmitting(true);
    const parsedCount = manualCount.trim() === '' ? undefined : Number(manualCount);
    await onSubmit(category, parsedCount, category === 'other' ? note : undefined);
    setSubmitting(false);
  };

  return (
    <ConfirmDialog
      title={`Dogfood feedback — application ${ordinal} of ${targetCount}`}
      titleId="dogfood-feedback-title"
      onCancel={onSkip}
      actions={
        // Deliberately not ConfirmActions: its shared `disabled` prop would
        // gate Cancel along with Confirm, and Skip must stay enabled even
        // before a category is chosen -- feedback is always skippable.
        <>
          <ConsoleButton variant="primary" onClick={handleSubmit} disabled={!category || submitting}>
            Submit
          </ConsoleButton>
          <ConsoleButton variant="ghost" onClick={onSkip} disabled={submitting}>
            Skip
          </ConsoleButton>
        </>
      }
    >
      <p className="confirm-subject">{subject}</p>
      <p>What slowed you down on this application?</p>
      <fieldset className="dogfood-feedback-options">
        <legend className="visually-hidden">Feedback category</legend>
        {OPTIONS.map((option) => (
          <label key={option.value} className="dogfood-feedback-option">
            <input
              type="radio"
              name="dogfood-feedback-category"
              value={option.value}
              checked={category === option.value}
              onChange={() => setCategory(option.value)}
            />
            {option.label}
          </label>
        ))}
      </fieldset>
      {category === 'other' && (
        <label className="dogfood-feedback-note">
          Optional note — keep it general, no company names or answers
          <textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} rows={2} />
        </label>
      )}
      <label className="dogfood-feedback-manual-count">
        Manual fields/questions you had to handle (optional)
        <input type="number" min={0} value={manualCount} onChange={(e) => setManualCount(e.target.value)} />
      </label>
    </ConfirmDialog>
  );
}
