import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { DogfoodFeedbackDialog } from './DogfoodFeedbackDialog';

describe('DogfoodFeedbackDialog', () => {
  it('renders all eight feedback options', () => {
    render(
      <DogfoodFeedbackDialog ordinal={2} targetCount={5} subject="Acme — Engineer" onSubmit={vi.fn()} onSkip={vi.fn()} />
    );
    for (const label of [
      'Nothing meaningful',
      'Bad/irrelevant job match',
      'Known information was not filled',
      'Career Agent filled something incorrectly',
      'Repeated question Career Agent should know',
      'One-off employer question',
      'Browser/application blocker',
      'Other',
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it('disables Submit until a category is chosen', () => {
    render(<DogfoodFeedbackDialog ordinal={1} targetCount={5} subject="Acme — Engineer" onSubmit={vi.fn()} onSkip={vi.fn()} />);
    expect(screen.getByRole('button', { name: /submit/i })).toBeDisabled();
    fireEvent.click(screen.getByLabelText('Nothing meaningful'));
    expect(screen.getByRole('button', { name: /submit/i })).not.toBeDisabled();
  });

  it('calls onSkip without calling onSubmit when skipped, even with no category chosen', () => {
    const onSubmit = vi.fn();
    const onSkip = vi.fn();
    render(<DogfoodFeedbackDialog ordinal={1} targetCount={5} subject="Acme — Engineer" onSubmit={onSubmit} onSkip={onSkip} />);
    expect(screen.getByRole('button', { name: /skip/i })).not.toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /skip/i }));
    expect(onSkip).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('only shows the optional note field once Other is selected', () => {
    render(<DogfoodFeedbackDialog ordinal={1} targetCount={5} subject="Acme — Engineer" onSubmit={vi.fn()} onSkip={vi.fn()} />);
    expect(screen.queryByText(/optional note/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Other'));
    expect(screen.getByText(/optional note/i)).toBeInTheDocument();
  });

  it('submits the chosen category, an optional manual count, and no note for a non-Other category', async () => {
    const onSubmit = vi.fn().mockResolvedValue(true);
    render(<DogfoodFeedbackDialog ordinal={1} targetCount={5} subject="Acme — Engineer" onSubmit={onSubmit} onSkip={vi.fn()} />);
    fireEvent.click(screen.getByLabelText('Repeated question Career Agent should know'));
    fireEvent.change(screen.getByLabelText(/manual fields\/questions/i), { target: { value: '2' } });
    fireEvent.click(screen.getByRole('button', { name: /submit/i }));
    expect(onSubmit).toHaveBeenCalledWith('repeated_question', 2, undefined);
  });

  it('submits the note only when Other is selected', async () => {
    const onSubmit = vi.fn().mockResolvedValue(true);
    render(<DogfoodFeedbackDialog ordinal={1} targetCount={5} subject="Acme — Engineer" onSubmit={onSubmit} onSkip={vi.fn()} />);
    fireEvent.click(screen.getByLabelText('Other'));
    fireEvent.change(screen.getByText(/optional note/i).closest('label')!.querySelector('textarea')!, {
      target: { value: 'the site timed out' },
    });
    fireEvent.click(screen.getByRole('button', { name: /submit/i }));
    expect(onSubmit).toHaveBeenCalledWith('other', undefined, 'the site timed out');
  });
});
