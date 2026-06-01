"use client";

import type React from "react";
import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";
import { reviewQuestions } from "@/lib/pilot-form-questions";
import { useParams } from "next/navigation";

const questionLabel = Object.fromEntries(reviewQuestions.map((question) => [question.key, question.label])) as Record<string, string>;

// ReviewInfo stores the fields used by pilot forms.
type ReviewInfo = {
  name: string;
  issue_count: number;
};

// PilotReviewPage renders the pilot review page for pilot forms.
export default function PilotReviewPage() {
  const params = useParams<{ token: string }>();
  const token = params.token;
  const [form, setForm] = useState({
    would_keep_using: "",
    keep_using_reason: "",
    most_important_features: "",
    not_keep_using_reason: "",
    switch_requirements: "",
    open_to_follow_up: "",
  });
  const [reviewInfo, setReviewInfo] = useState<ReviewInfo | null>(null);
  const [status, setStatus] = useState<"idle" | "submitting" | "submitted" | "error">("idle");
  const [message, setMessage] = useState("");

  useEffect(() => {
    let cancelled = false;
    // loadReviewInfo loads review info for pilot forms.
    async function loadReviewInfo() {
      if (!token) return;
      try {
        const response = await fetch(`${API_URL}/api/v1/pilot/review/${encodeURIComponent(token)}`);
        if (!response.ok) return;
        const result = await response.json() as ReviewInfo;
        if (!cancelled) setReviewInfo(result);
      } catch {
        // Keep the form usable even if the review metadata cannot be loaded.
      }
    }
    void loadReviewInfo();
    return () => {
      cancelled = true;
    };
  }, [token]);

  // submit submits the current form or feedback request.
  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setStatus("submitting");
    setMessage("");
    try {
      const response = await fetch(`${API_URL}/api/v1/pilot/review/${encodeURIComponent(token)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(form),
      });
      if (!response.ok) throw new Error(await response.text());
      setStatus("submitted");
      setMessage("Thanks. Your review has been submitted.");
    } catch (error) {
      setStatus("error");
      setMessage(error instanceof Error ? error.message : "Could not submit review.");
    }
  }

  return (
    <main style={pageStyle}>
      <form style={formStyle} onSubmit={submit}>
        <div style={headerBlockStyle}>
          <div style={eyebrowStyle}>Pilot review</div>
          <h1 style={titleStyle}>Review your experience</h1>
          <p style={copyStyle}>{introCopy(reviewInfo)}</p>
        </div>

        <section style={sectionStyle}>
          <Field label={questionLabel.would_keep_using}>
            <div style={buttonRowStyle}>
              {(["yes", "no"] as const).map((value) => (
                <button
                  key={value}
                  type="button"
                  style={form.would_keep_using === value ? selectedOptionStyle : optionStyle}
                  onClick={() => setForm({ ...form, would_keep_using: value })}
                >
                  {value === "yes" ? "Yes" : "No"}
                </button>
              ))}
            </div>
          </Field>

          {form.would_keep_using === "no" && (
            <>
              <Field label={questionLabel.not_keep_using_reason}>
                <textarea style={textareaStyle} value={form.not_keep_using_reason} onChange={(event) => setForm({ ...form, not_keep_using_reason: event.target.value })} />
              </Field>
              <Field label={questionLabel.switch_requirements}>
                <textarea style={textareaStyle} value={form.switch_requirements} onChange={(event) => setForm({ ...form, switch_requirements: event.target.value })} />
              </Field>
            </>
          )}

          {form.would_keep_using === "yes" && (
            <>
              <Field label={questionLabel.keep_using_reason}>
                <textarea style={textareaStyle} value={form.keep_using_reason} onChange={(event) => setForm({ ...form, keep_using_reason: event.target.value })} />
              </Field>
              <Field label={questionLabel.most_important_features}>
                <textarea style={textareaStyle} value={form.most_important_features} onChange={(event) => setForm({ ...form, most_important_features: event.target.value })} />
              </Field>
            </>
          )}

          <Field label={questionLabel.open_to_follow_up}>
            <div style={buttonRowStyle}>
              {(["yes", "no"] as const).map((value) => (
                <button
                  key={value}
                  type="button"
                  style={form.open_to_follow_up === value ? selectedOptionStyle : optionStyle}
                  onClick={() => setForm({ ...form, open_to_follow_up: value })}
                >
                  {value === "yes" ? "Yes" : "No"}
                </button>
              ))}
            </div>
          </Field>
        </section>

        <div style={footerStyle}>
          <button style={primaryButtonStyle} disabled={status === "submitting" || !token}>{status === "submitting" ? "Submitting..." : "Submit review"}</button>
          {message && <div style={status === "error" ? errorStyle : successStyle}>{message}</div>}
        </div>
      </form>
    </main>
  );
}

// introCopy builds the review-form intro copy from pilot progress.
function introCopy(reviewInfo: ReviewInfo | null) {
  if (!reviewInfo) {
    return "Thanks for being an early tester. We'd like to ask a few questions about your experience so far.";
  }
  const issueLabel = reviewInfo.issue_count === 1 ? "issue" : "issues";
  return `Thanks for being an early tester ${reviewInfo.name}. You reported ${reviewInfo.issue_count} ${issueLabel}. We'd like to ask a few questions about your experience so far.`;
}

// Field renders a labeled form field.
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label style={fieldStyle}>
      <span style={labelStyle}>{label}</span>
      {children}
    </label>
  );
}

const pageStyle: React.CSSProperties = { minHeight: "100vh", background: "#EBE9E9", color: "#111", padding: "34px 24px 48px", display: "grid", alignItems: "center" };
const formStyle: React.CSSProperties = { width: "min(820px, 100%)", margin: "0 auto", display: "grid", gap: 14 };
const headerBlockStyle: React.CSSProperties = { padding: "6px 0 4px" };
const eyebrowStyle: React.CSSProperties = { color: "#777", fontSize: 11, fontWeight: 750, textTransform: "uppercase", marginBottom: 7 };
const titleStyle: React.CSSProperties = { margin: 0, fontSize: 24, lineHeight: 1.18, fontWeight: 750 };
const copyStyle: React.CSSProperties = { margin: "8px 0 0", color: "#666", fontSize: 14, lineHeight: 1.5 };
const sectionStyle: React.CSSProperties = { border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFFFFF", padding: 18, display: "grid", gap: 12 };
const fieldStyle: React.CSSProperties = { display: "grid", gap: 7 };
const labelStyle: React.CSSProperties = { color: "#111111", fontSize: 13, fontWeight: 700, lineHeight: 1.35 };
const buttonRowStyle: React.CSSProperties = { display: "flex", gap: 8, flexWrap: "wrap" };
const optionStyle: React.CSSProperties = { height: 36, borderWidth: 1, borderStyle: "solid", borderColor: "#D4D4D4", borderRadius: 6, background: "#FFF", color: "#111", padding: "0 14px", cursor: "pointer", fontWeight: 700, fontSize: 13 };
const selectedOptionStyle: React.CSSProperties = { ...optionStyle, borderColor: "#111", background: "#111", color: "#FFF" };
const inputStyle: React.CSSProperties = { height: 42, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", fontSize: 14 };
const textareaStyle: React.CSSProperties = { ...inputStyle, height: 92, padding: 11, resize: "vertical", lineHeight: 1.45 };
const footerStyle: React.CSSProperties = { display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" };
const primaryButtonStyle: React.CSSProperties = { height: 40, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 14px", cursor: "pointer", fontWeight: 700, fontSize: 13 };
const statusMessageStyle: React.CSSProperties = { maxWidth: 360, marginLeft: "auto", padding: 12, borderRadius: 8, fontSize: 13, lineHeight: 1.4 };
const successStyle: React.CSSProperties = { ...statusMessageStyle, border: "1px solid #BFE2C5", background: "#EEF8F0", color: "#225B2D" };
const errorStyle: React.CSSProperties = { ...statusMessageStyle, border: "1px solid #F3B4B4", background: "#FFF1F1", color: "#991B1B" };
