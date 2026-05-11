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
        <div>
          <div style={eyebrowStyle}>Isoprism pilot</div>
          <h1 style={titleStyle}>Pilot review</h1>
        </div>
        <input style={inputStyle} type="number" min={1} max={5} placeholder="Isoprism helped me understand PRs faster (1-5)" value={form.faster_rating} onChange={(event) => setForm({ ...form, faster_rating: event.target.value })} />
        <input style={inputStyle} type="number" min={1} max={5} placeholder="The graph made review risk clearer (1-5)" value={form.risk_clarity_rating} onChange={(event) => setForm({ ...form, risk_clarity_rating: event.target.value })} />
        <textarea style={textareaStyle} placeholder="What was confusing or missing?" value={form.confusing_or_missing} onChange={(event) => setForm({ ...form, confusing_or_missing: event.target.value })} />
        <textarea style={textareaStyle} placeholder="What bugs did you hit?" value={form.bugs_hit} onChange={(event) => setForm({ ...form, bugs_hit: event.target.value })} />
        <textarea style={textareaStyle} placeholder="What should we build next?" value={form.build_next} onChange={(event) => setForm({ ...form, build_next: event.target.value })} />
        <input style={inputStyle} placeholder="Would you keep using Isoprism for PR review?" value={form.would_keep_using} onChange={(event) => setForm({ ...form, would_keep_using: event.target.value })} />
        <button style={primaryButtonStyle} disabled={status === "submitting" || !token}>{status === "submitting" ? "Submitting..." : "Submit review"}</button>
        {message && <div style={status === "error" ? errorStyle : successStyle}>{message}</div>}
      </form>
    </main>
  );
}

const pageStyle: React.CSSProperties = { minHeight: "100vh", background: "#EBE9E9", color: "#111", padding: "42px 20px" };
const formStyle: React.CSSProperties = { width: "min(680px, 100%)", margin: "0 auto", display: "grid", gap: 12 };
const eyebrowStyle: React.CSSProperties = { color: "#777", fontSize: 12, fontWeight: 750, textTransform: "uppercase", marginBottom: 6 };
const titleStyle: React.CSSProperties = { margin: 0, fontSize: 32, lineHeight: 1.12 };
const inputStyle: React.CSSProperties = { height: 42, border: "1px solid #D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", fontSize: 14 };
const textareaStyle: React.CSSProperties = { ...inputStyle, height: 96, padding: 11, resize: "vertical" };
const primaryButtonStyle: React.CSSProperties = { height: 42, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 16px", cursor: "pointer", fontWeight: 700 };
const successStyle: React.CSSProperties = { border: "1px solid #BFE2C5", borderRadius: 8, background: "#EEF8F0", color: "#225B2D", padding: 12 };
const errorStyle: React.CSSProperties = { border: "1px solid #F3B4B4", borderRadius: 8, background: "#FFF1F1", color: "#991B1B", padding: 12 };
