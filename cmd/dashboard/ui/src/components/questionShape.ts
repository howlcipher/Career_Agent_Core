/**
 * The shape of one application question, and the pure functions that read it.
 *
 * Separate from QuestionInput.tsx only because a file that exports both
 * components and helpers defeats fast refresh. The reasoning behind each rule
 * lives with the rule.
 */

export interface QuestionShape {
  /** The employer's wording, or Career Agent's canonical wording for a group. */
  prompt: string;
  control_type: string;
  options?: string[] | null;
  required?: boolean;
  sensitivity: string;
  suggested?: string;
  label_unsafe?: boolean;
}

export interface ReuseDecision {
  saveForReuse: boolean;
  allowSensitiveReuse: boolean;
  scope: string;
}

export const isSensitive = (question: QuestionShape) => question.sensitivity === 'sensitive';
export const isGenerated = (question: QuestionShape) => question.sensitivity === 'generate_per_job';

/**
 * Yes/no is by far the most common shape of a screening question, and a
 * two-button answer is one click where a text box is a click plus typing.
 * Only offered when the control genuinely is a choice and the discovered
 * options look like a yes/no pair — never inferred from the question's wording,
 * which is how a "no" ends up recorded against a question that meant something
 * else.
 */
export const yesNoOptions = (question: QuestionShape): string[] | null => {
  const options = question.options ?? [];
  if (options.length !== 2) return null;
  const lowered = options.map((option) => option.trim().toLowerCase());
  if (lowered.includes('yes') && lowered.includes('no')) return options;
  return null;
};
