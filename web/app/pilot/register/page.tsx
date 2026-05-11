"use client";

import type React from "react";
import { useState } from "react";
import { API_URL } from "@/lib/api";

export default function PilotRegisterPage() {
  const [form, setForm] = useState({
    ai_writes_most_software: "",
    current_review_tools: "",
    review_work_percent: 20,
    role_change: "",
    review_pain_points: "",
    ai_review_difference: "",
    other_comments: "",
    interested_in_pilot: "",
    name: "",
    email: "",
    pilot_languages: "",
    public_repo_url: "",
  });
  const [status, setStatus] = useState<"idle" | "submitting" | "submitted" | "error">("idle");
  const [message, setMessage] = useState("");

  const interested = form.interested_in_pilot === "yes";

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setStatus("submitting");
    setMessage("");
    try {
      const response = await fetch(`${API_URL}/api/v1/pilot/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...form,
          interested_in_pilot: interested,
          review_work_percent: Number(form.review_work_percent),
        }),
      });
      if (!response.ok) throw new Error(await response.text());
      setStatus("submitted");
      setMessage("Thanks. Your response has been saved.");
    } catch (error) {
      setStatus("error");
      setMessage(error instanceof Error ? error.message : "Could not submit registration.");
    }
  }

  return (
    <main style={pageStyle}>
      <form style={formStyle} onSubmit={submit}>
        <div>
          <div style={eyebrowStyle}>Isoprism pilot</div>
          <h1 style={titleStyle}>Register interest</h1>
          <p style={copyStyle}>A short survey about how engineers understand code changes as AI writes more software.</p>
        </div>

        <Field label="Does AI write most of your software?">
          <Segmented value={form.ai_writes_most_software} options={["Yes", "No"]} onChange={(value) => setForm({ ...form, ai_writes_most_software: value })} />
        </Field>

        <Field label="What do you use currently to review software?">
          <Segmented value={form.current_review_tools} options={["Github", "IDE", "Other"]} onChange={(value) => setForm({ ...form, current_review_tools: value })} />
        </Field>

        <Field label="How much of your work is spent reviewing code?">
          <input style={inputStyle} type="number" min={0} max={100} value={form.review_work_percent} onChange={(event) => setForm({ ...form, review_work_percent: Number(event.target.value) })} />
        </Field>

        <Textarea label="What has changed in your role as an engineer in the last 12 months?" value={form.role_change} onChange={(value) => setForm({ ...form, role_change: value })} />
        <Textarea label="What pain points, if any, do you currently face in reviewing software?" value={form.review_pain_points} onChange={(value) => setForm({ ...form, review_pain_points: value })} />
        <Textarea label="Do you review software written by AI any differently to humans, if so how?" value={form.ai_review_difference} onChange={(value) => setForm({ ...form, ai_review_difference: value })} />
        <Textarea label="Any other comments that would be valuable on understanding code changes?" value={form.other_comments} onChange={(value) => setForm({ ...form, other_comments: value })} />

        <section style={pilotBoxStyle}>
          <Field label="Would you be interested in piloting a prototype to help engineers understand how AI has built your software?">
            <Segmented value={form.interested_in_pilot} options={["Yes", "No"]} onChange={(value) => setForm({ ...form, interested_in_pilot: value.toLowerCase() })} />
          </Field>
          <p style={copyStyle}>The pilot is one week using Isoprism with one repository. You will connect GitHub, choose one repo, use the prototype during PR review, and complete a short review at the end.</p>
          {interested && (
            <div style={detailsGridStyle}>
              <input style={inputStyle} required placeholder="Name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
              <input style={inputStyle} required type="email" placeholder="Email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
              <input style={inputStyle} placeholder="Language/s for the pilot" value={form.pilot_languages} onChange={(event) => setForm({ ...form, pilot_languages: event.target.value })} />
              <input style={inputStyle} placeholder="Public repo link, if public" value={form.public_repo_url} onChange={(event) => setForm({ ...form, public_repo_url: event.target.value })} />
            </div>
          )}
        </section>

        <button style={primaryButtonStyle} disabled={status === "submitting"}>{status === "submitting" ? "Submitting..." : "Submit"}</button>
        {message && <div style={status === "error" ? errorStyle : successStyle}>{message}</div>}
      </form>
    </main>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label style={fieldStyle}><span style={labelStyle}>{label}</span>{children}</label>;
}

function Textarea({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <Field label={label}><textarea style={textareaStyle} value={value} onChange={(event) => onChange(event.target.value)} /></Field>;
}

function Segmented({ value, options, onChange }: { value: string; options: string[]; onChange: (value: string) => void }) {
  return <div style={segmentedStyle}>{options.map((option) => <button key={option} type="button" style={value.toLowerCase() === option.toLowerCase() ? activeSegmentStyle : segmentStyle} onClick={() => onChange(option)}>{option}</button>)}</div>;
}

const pageStyle: React.CSSProperties = { minHeight: "100vh", background: "#EBE9E9", color: "#111", padding: "42px 20px" };
const formStyle: React.CSSProperties = { width: "min(760px, 100%)", margin: "0 auto", display: "grid", gap: 18 };
const eyebrowStyle: React.CSSProperties = { color: "#777", fontSize: 12, fontWeight: 750, textTransform: "uppercase", marginBottom: 6 };
const titleStyle: React.CSSProperties = { margin: 0, fontSize: 32, lineHeight: 1.12 };
const copyStyle: React.CSSProperties = { color: "#666", fontSize: 14, lineHeight: 1.55, margin: "6px 0 0" };
const fieldStyle: React.CSSProperties = { display: "grid", gap: 7 };
const labelStyle: React.CSSProperties = { fontSize: 13, fontWeight: 700 };
const inputStyle: React.CSSProperties = { height: 42, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", fontSize: 14 };
const textareaStyle: React.CSSProperties = { ...inputStyle, height: 96, padding: 11, resize: "vertical" };
const segmentedStyle: React.CSSProperties = { display: "flex", flexWrap: "wrap", gap: 8 };
const segmentStyle: React.CSSProperties = { height: 36, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 12px", cursor: "pointer" };
const activeSegmentStyle: React.CSSProperties = { ...segmentStyle, background: "#111", color: "#FFF", borderColor: "#111" };
const pilotBoxStyle: React.CSSProperties = { border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFF", padding: 16, display: "grid", gap: 12 };
const detailsGridStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 8 };
const primaryButtonStyle: React.CSSProperties = { height: 42, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 16px", cursor: "pointer", fontWeight: 700 };
const successStyle: React.CSSProperties = { border: "1px solid #BFE2C5", borderRadius: 8, background: "#EEF8F0", color: "#225B2D", padding: 12 };
const errorStyle: React.CSSProperties = { border: "1px solid #F3B4B4", borderRadius: 8, background: "#FFF1F1", color: "#991B1B", padding: 12 };
