// PilotFormType describes pilot form data used by web API utilities.
export type PilotFormType = "registration" | "review";

// PilotFormQuestion describes pilot form data used by web API utilities.
export type PilotFormQuestion = {
  key: string;
  label: string;
  askedWhen?: (answers: Record<string, unknown>) => boolean;
};

export const registrationQuestions: PilotFormQuestion[] = [
  {
    key: "software_experience",
    label: "How many years experience do you have with software?",
  },
  {
    key: "ai_software_exceptions",
    label: "Are there any parts of your software that isn't written by AI, if so which?",
  },
  {
    key: "issue_prompt_process",
    label: "If you are assigned an issue, what process do you have leading up to generating your prompt/s?",
  },
  {
    key: "current_review_tools",
    label: "What do you use currently to review software?",
  },
  {
    key: "review_work_percent",
    label: "How much of your time is spent reviewing code?",
  },
  {
    key: "review_pain_points",
    label: "What pain points, if any, do you currently face in reviewing software?",
  },
  {
    key: "ai_review_usage",
    label: "How do you use AI to review software?",
  },
  {
    key: "ai_resolved_issue_types",
    label: "Which type of issues, if any, do you or would you have resolved without any human in the loop?",
  },
  {
    key: "ai_review_difference",
    label: "Do you review software written by AI any differently to humans, if so how?",
  },
  {
    key: "interested_in_pilot",
    label: "Would you be interested in piloting a prototype aiming at helping engineers understand the systems that AI builds?",
  },
  {
    key: "name",
    label: "Name",
    askedWhen: (answers) => Boolean(answers.interested_in_pilot),
  },
  {
    key: "email",
    label: "Email",
    askedWhen: (answers) => Boolean(answers.interested_in_pilot),
  },
  {
    key: "pilot_languages",
    label: "Language/s for the pilot",
    askedWhen: (answers) => Boolean(answers.interested_in_pilot),
  },
  {
    key: "public_repo_url",
    label: "Public repo link, if public",
    askedWhen: (answers) => Boolean(answers.interested_in_pilot),
  },
];

export const reviewQuestions: PilotFormQuestion[] = [
  {
    key: "would_keep_using",
    label: "Would you keep using Isoprism for PR reviews over your existing flow?",
  },
  {
    key: "not_keep_using_reason",
    label: "Why not?",
    askedWhen: (answers) => answers.would_keep_using === "no",
  },
  {
    key: "switch_requirements",
    label: "What would it take for you to switch from your current flow?",
    askedWhen: (answers) => answers.would_keep_using === "no",
  },
  {
    key: "keep_using_reason",
    label: "Why? Tell us what you liked about it",
    askedWhen: (answers) => answers.would_keep_using === "yes",
  },
  {
    key: "most_important_features",
    label: "What do you think is missing? What would you like to be able to do but can't?",
    askedWhen: (answers) => answers.would_keep_using === "yes",
  },
  {
    key: "open_to_follow_up",
    label: "Would you be open to us reaching out to get a better understanding of your experience or for trialling future versions?",
  },
];

// questionsForForm returns the configured question list for a pilot form type.
export function questionsForForm(type: PilotFormType) {
  return type === "registration" ? registrationQuestions : reviewQuestions;
}
