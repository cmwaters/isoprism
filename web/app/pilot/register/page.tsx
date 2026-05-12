"use client";

import type React from "react";
import { useState } from "react";
import { API_URL } from "@/lib/api";

export default function PilotRegisterPage() {
  const [form, setForm] = useState({
    software_experience: "",
    ai_writes_most_software: "",
    current_review_tools: "",
    review_work_percent: 20,
    review_pain_points: "",
    ai_review_difference: "",
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
        <div style={headerBlockStyle}>
          <div style={eyebrowStyle}>Pilot registration</div>
          <h1 style={titleStyle}>Register interest</h1>
          <p style={copyStyle}>We&apos;d like to better understand how AI has changed the workflows of software engineers so we can better rethink how new workflows should look like.</p>
        </div>

        <section style={sectionStyle}>
          <h2 style={sectionTitleStyle}>Review Experience</h2>
          <div style={stackStyle}>
            <Field label="How many years experience do you have with software?">
              <Segmented value={form.software_experience} options={["None", "<2 years", "2-5 years", "6-10 years", "10+ years"]} onChange={(value) => setForm({ ...form, software_experience: value })} />
            </Field>

            <Field label="Does AI write most of your software?">
              <Segmented value={form.ai_writes_most_software} options={["Yes", "No"]} onChange={(value) => setForm({ ...form, ai_writes_most_software: value })} />
            </Field>

            <Field label="What do you use currently to review software?">
              <div style={toolRowStyle}>
                <Segmented value={form.current_review_tools} options={["Github", "IDE"]} onChange={(value) => setForm({ ...form, current_review_tools: value })} />
                <input
                  style={toolInputStyle}
                  placeholder="Something else..."
                  value={form.current_review_tools === "Github" || form.current_review_tools === "IDE" ? "" : form.current_review_tools}
                  onChange={(event) => setForm({ ...form, current_review_tools: event.target.value })}
                />
              </div>
            </Field>

            <Field label="How much of your work is spent reviewing code?">
              <div style={rangeRowStyle}>
                <input
                  aria-label="Review work rating"
                  style={rangeStyle}
                  type="range"
                  min={0}
                  max={100}
                  step={1}
                  value={form.review_work_percent}
                  onChange={(event) => setForm({ ...form, review_work_percent: Number(event.target.value) })}
                />
                <span style={rangeValueStyle}>{form.review_work_percent}%</span>
              </div>
            </Field>
          </div>

          <Textarea label="What pain points, if any, do you currently face in reviewing software?" value={form.review_pain_points} onChange={(value) => setForm({ ...form, review_pain_points: value })} />
          <Textarea label="Do you review software written by AI any differently to humans, if so how?" value={form.ai_review_difference} onChange={(value) => setForm({ ...form, ai_review_difference: value })} />
        </section>

        <section style={sectionStyle}>
          <h2 style={sectionTitleStyle}>Pilot interest</h2>
          <Field label="Would you be interested in piloting a prototype aiming at helping engineers understand the systems that AI builds?">
            <Segmented value={form.interested_in_pilot} options={["Yes", "No"]} onChange={(value) => setForm({ ...form, interested_in_pilot: value.toLowerCase() })} />
          </Field>
          <p style={copyStyle}>The pilot is one week using Isoprism with one repository. You are expected to connect to GitHub, choose one repo, use the prototype during PR review, and complete a short review at the end.</p>
          {interested && (
            <div style={detailsGridStyle}>
              <input style={inputStyle} required placeholder="Name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
              <input style={inputStyle} required type="email" placeholder="Email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
              <input style={inputStyle} placeholder="Language/s for the pilot" value={form.pilot_languages} onChange={(event) => setForm({ ...form, pilot_languages: event.target.value })} />
              <input style={inputStyle} placeholder="Public repo link, if public" value={form.public_repo_url} onChange={(event) => setForm({ ...form, public_repo_url: event.target.value })} />
            </div>
          )}
        </section>

        <div style={footerStyle}>
          {message && <div style={status === "error" ? errorStyle : successStyle}>{message}</div>}
          <button style={primaryButtonStyle} disabled={status === "submitting"}>{status === "submitting" ? "Submitting..." : "Submit registration"}</button>
        </div>
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

const pageStyle: React.CSSProperties = { minHeight: "100vh", background: "#EBE9E9", color: "#111", padding: "34px 24px 48px", display: "grid", alignItems: "center" };
const formStyle: React.CSSProperties = { width: "min(920px, 100%)", margin: "0 auto", display: "grid", gap: 14 };
const headerBlockStyle: React.CSSProperties = { padding: "6px 0 4px" };
const eyebrowStyle: React.CSSProperties = { color: "#777", fontSize: 11, fontWeight: 750, textTransform: "uppercase", marginBottom: 7 };
const titleStyle: React.CSSProperties = { margin: 0, fontSize: 24, lineHeight: 1.18, fontWeight: 750 };
const copyStyle: React.CSSProperties = { color: "#666", fontSize: 14, lineHeight: 1.55, margin: "6px 0 0" };
const sectionStyle: React.CSSProperties = { border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFFFFF", padding: 18, display: "grid", gap: 14 };
const sectionTitleStyle: React.CSSProperties = { margin: 0, color: "#111111", fontSize: 15, fontWeight: 750 };
const stackStyle: React.CSSProperties = { display: "grid", gap: 14 };
const fieldStyle: React.CSSProperties = { display: "grid", gap: 7 };
const labelStyle: React.CSSProperties = { color: "#333333", fontSize: 13, fontWeight: 700, lineHeight: 1.35 };
const inputStyle: React.CSSProperties = { height: 42, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", fontSize: 14 };
const textareaStyle: React.CSSProperties = { ...inputStyle, height: 92, padding: 11, resize: "vertical", lineHeight: 1.45 };
const segmentedStyle: React.CSSProperties = { display: "flex", flexWrap: "wrap", gap: 8 };
const segmentStyle: React.CSSProperties = { height: 34, borderWidth: 1, borderStyle: "solid", borderColor: "#D4D4D4", borderRadius: 6, background: "#FAFAFA", color: "#333333", padding: "0 12px", cursor: "pointer", fontSize: 13, fontWeight: 650 };
const activeSegmentStyle: React.CSSProperties = { ...segmentStyle, background: "#111", color: "#FFF", borderWidth: 1, borderStyle: "solid", borderColor: "#111" };
const rangeRowStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "minmax(180px, 1fr) auto", alignItems: "center", gap: 12 };
const rangeStyle: React.CSSProperties = { width: "100%", accentColor: "#111111", cursor: "pointer" };
const rangeValueStyle: React.CSSProperties = { minWidth: 34, color: "#333333", fontSize: 13, fontWeight: 700, textAlign: "right" };
const toolRowStyle: React.CSSProperties = { display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" };
const toolInputStyle: React.CSSProperties = { ...inputStyle, flex: "1 1 220px", minWidth: 0 };
const detailsGridStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 8 };
const footerStyle: React.CSSProperties = { display: "grid", justifyItems: "end", gap: 10 };
const primaryButtonStyle: React.CSSProperties = { height: 40, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 14px", cursor: "pointer", fontWeight: 700, fontSize: 13 };
const statusMessageStyle: React.CSSProperties = { maxWidth: 360, padding: "10px 12px", borderRadius: 8, fontSize: 13, lineHeight: 1.4, textAlign: "right" };
const successStyle: React.CSSProperties = { ...statusMessageStyle, border: "1px solid #BFE2C5", background: "#EEF8F0", color: "#225B2D" };
const errorStyle: React.CSSProperties = { ...statusMessageStyle, border: "1px solid #F3B4B4", background: "#FFF1F1", color: "#991B1B" };
