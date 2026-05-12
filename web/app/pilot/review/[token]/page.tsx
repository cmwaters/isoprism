"use client";

import type React from "react";
import { useState } from "react";
import { API_URL } from "@/lib/api";
import { useParams } from "next/navigation";

export default function PilotReviewPage() {
  const params = useParams<{ token: string }>();
  const token = params.token;
  const [form, setForm] = useState({
    faster_rating: "",
    risk_clarity_rating: "",
    confusing_or_missing: "",
    bugs_hit: "",
    build_next: "",
    would_keep_using: "",
    keep_using_reason: "",
    most_important_features: "",
    fair_cost: "",
    not_keep_using_reason: "",
    switch_requirements: "",
  });
  const [status, setStatus] = useState<"idle" | "submitting" | "submitted" | "error">("idle");
  const [message, setMessage] = useState("");

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setStatus("submitting");
    setMessage("");
    try {
      const response = await fetch(`${API_URL}/api/v1/pilot/review/${encodeURIComponent(token)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...form,
          faster_rating: form.faster_rating ? Number(form.faster_rating) : null,
          risk_clarity_rating: form.risk_clarity_rating ? Number(form.risk_clarity_rating) : null,
        }),
      });
      if (!response.ok) throw new Error(await response.text());
      setStatus("submitted");
      setMessage("Thanks. Your review has been saved.");
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
          <h1 style={titleStyle}>Share your review experience</h1>
        </div>

        <section style={sectionStyle}>
          <Field label="Would you keep using Isoprism for PR reviews over your existing flow?">
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
              <Field label="Why not?">
                <textarea style={textareaStyle} value={form.not_keep_using_reason} onChange={(event) => setForm({ ...form, not_keep_using_reason: event.target.value })} />
              </Field>
              <Field label="What would it take for you to switch from your current flow?">
                <textarea style={textareaStyle} value={form.switch_requirements} onChange={(event) => setForm({ ...form, switch_requirements: event.target.value })} />
              </Field>
            </>
          )}

          {form.would_keep_using === "yes" && (
            <>
              <Field label="Why?">
                <textarea style={textareaStyle} value={form.keep_using_reason} onChange={(event) => setForm({ ...form, keep_using_reason: event.target.value })} />
              </Field>
              <Field label="What do you think are the most important features that should be added?">
                <textarea style={textareaStyle} value={form.most_important_features} onChange={(event) => setForm({ ...form, most_important_features: event.target.value })} />
              </Field>
              <Field label="If this were a paid product, what would you consider a fair cost?">
                <input style={inputStyle} value={form.fair_cost} onChange={(event) => setForm({ ...form, fair_cost: event.target.value })} />
              </Field>
            </>
          )}

          <h2 style={sectionTitleStyle}>Review impact</h2>
          <div style={twoColumnStyle}>
            <Field label="Understood PRs faster (1-5)">
              <input style={inputStyle} type="number" min={1} max={5} value={form.faster_rating} onChange={(event) => setForm({ ...form, faster_rating: event.target.value })} />
            </Field>
            <Field label="Review risk clearer (1-5)">
              <input style={inputStyle} type="number" min={1} max={5} value={form.risk_clarity_rating} onChange={(event) => setForm({ ...form, risk_clarity_rating: event.target.value })} />
            </Field>
          </div>

          <h2 style={sectionTitleStyle}>Pilot notes</h2>
          <Field label="What was confusing or missing?">
            <textarea style={textareaStyle} value={form.confusing_or_missing} onChange={(event) => setForm({ ...form, confusing_or_missing: event.target.value })} />
          </Field>
          <Field label="What bugs did you hit?">
            <textarea style={textareaStyle} value={form.bugs_hit} onChange={(event) => setForm({ ...form, bugs_hit: event.target.value })} />
          </Field>
          <Field label="What should we build next?">
            <textarea style={textareaStyle} value={form.build_next} onChange={(event) => setForm({ ...form, build_next: event.target.value })} />
          </Field>
        </section>

        <div style={footerStyle}>
          {message && <div style={status === "error" ? errorStyle : successStyle}>{message}</div>}
          <button style={primaryButtonStyle} disabled={status === "submitting" || !token}>{status === "submitting" ? "Submitting..." : "Submit review"}</button>
        </div>
      </form>
    </main>
  );
}

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
const sectionStyle: React.CSSProperties = { border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFFFFF", padding: 18, display: "grid", gap: 12 };
const sectionTitleStyle: React.CSSProperties = { margin: 0, color: "#111111", fontSize: 15, fontWeight: 750 };
const twoColumnStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 10 };
const fieldStyle: React.CSSProperties = { display: "grid", gap: 7 };
const labelStyle: React.CSSProperties = { color: "#111111", fontSize: 13, fontWeight: 700, lineHeight: 1.35 };
const buttonRowStyle: React.CSSProperties = { display: "flex", gap: 8, flexWrap: "wrap" };
const optionStyle: React.CSSProperties = { height: 36, borderWidth: 1, borderStyle: "solid", borderColor: "#D4D4D4", borderRadius: 6, background: "#FFF", color: "#111", padding: "0 14px", cursor: "pointer", fontWeight: 700, fontSize: 13 };
const selectedOptionStyle: React.CSSProperties = { ...optionStyle, borderColor: "#111", background: "#111", color: "#FFF" };
const inputStyle: React.CSSProperties = { height: 42, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", fontSize: 14 };
const textareaStyle: React.CSSProperties = { ...inputStyle, height: 92, padding: 11, resize: "vertical", lineHeight: 1.45 };
const footerStyle: React.CSSProperties = { display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" };
const primaryButtonStyle: React.CSSProperties = { height: 40, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 14px", cursor: "pointer", fontWeight: 700, fontSize: 13 };
const successStyle: React.CSSProperties = { border: "1px solid #BFE2C5", borderRadius: 8, background: "#EEF8F0", color: "#225B2D", padding: 12 };
const errorStyle: React.CSSProperties = { border: "1px solid #F3B4B4", borderRadius: 8, background: "#FFF1F1", color: "#991B1B", padding: 12 };
