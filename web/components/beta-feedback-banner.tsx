"use client";

import type React from "react";
import { useMemo, useState } from "react";
import { apiFetch } from "@/lib/api";
import {
  BetaFeedbackPayload,
  BetaFeedbackResponse,
  GraphNode,
  GraphPR,
  Repository,
} from "@/lib/types";

type FeedbackType = "bug" | "feature";

export default function BetaFeedbackBanner({
  token,
  repo,
  pr,
  selectedNode,
}: {
  token: string;
  repo: Repository;
  pr?: GraphPR;
  selectedNode?: GraphNode | null;
}) {
  const [feedbackType, setFeedbackType] = useState<FeedbackType | null>(null);
  const [title, setTitle] = useState("");
  const [details, setDetails] = useState("");
  const [status, setStatus] = useState<"idle" | "submitting" | "submitted" | "error">("idle");
  const [message, setMessage] = useState("");

  const appCommit = process.env.NEXT_PUBLIC_VERCEL_GIT_COMMIT_SHA ?? "local";
  const sourceCommit = pr?.head_commit_sha ?? repo.main_commit_sha ?? "";

  const contextLabel = useMemo(() => {
    const parts = [repo.full_name];
    if (pr) parts.push(`#${pr.number}`);
    if (selectedNode) parts.push(selectedNode.full_name);
    return parts.filter(Boolean).join(" · ");
  }, [pr, repo.full_name, selectedNode]);

  function open(type: FeedbackType) {
    setFeedbackType(type);
    setTitle("");
    setDetails("");
    setStatus("idle");
    setMessage("");
  }

  function close() {
    if (status === "submitting") return;
    setFeedbackType(null);
  }

  async function submit() {
    if (!feedbackType || !title.trim() || !details.trim()) return;

    setStatus("submitting");
    setMessage("");

    const payload: BetaFeedbackPayload = {
      type: feedbackType,
      title: title.trim(),
      details: details.trim(),
      repo_full_name: repo.full_name,
      repo_id: repo.id,
      pr_number: pr?.number,
      pr_title: pr?.title,
      node_full_name: selectedNode?.full_name,
      node_file_path: selectedNode?.file_path,
      browser_path: window.location.pathname,
      app_commit_sha: appCommit,
      source_commit_sha: sourceCommit,
    };

    try {
      const result = await apiFetch<BetaFeedbackResponse>("/api/v1/beta/feedback", token, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setStatus("submitted");
      setMessage(`Submitted as GitHub issue #${result.issue}.`);
    } catch (error) {
      setStatus("error");
      setMessage(error instanceof Error ? error.message : "Feedback could not be submitted.");
    }
  }

  return (
    <>
      <footer style={bannerStyle}>
        <span>This is a beta version of Isoprism.</span>
        <button style={bannerButtonStyle} onClick={() => open("bug")}>
          Report a problem
        </button>
        <span style={{ color: "#777777" }}>-</span>
        <button style={bannerButtonStyle} onClick={() => open("feature")}>
          Request a feature
        </button>
      </footer>

      {feedbackType && (
        <div style={overlayStyle} role="dialog" aria-modal="true" aria-label="Submit beta feedback">
          <div style={modalStyle}>
            <div style={modalHeaderStyle}>
              <div>
                <div style={eyebrowStyle}>{feedbackType === "bug" ? "Report a problem" : "Request a feature"}</div>
                <h2 style={titleStyle}>Send beta feedback</h2>
              </div>
              <button style={closeButtonStyle} onClick={close} aria-label="Close feedback form">
                ×
              </button>
            </div>

            <div style={contextStyle}>{contextLabel || "Current review context"}</div>

            <label style={labelStyle}>
              Title
              <input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                style={inputStyle}
                placeholder={feedbackType === "bug" ? "What broke?" : "What would help?"}
              />
            </label>

            <label style={labelStyle}>
              Details
              <textarea
                value={details}
                onChange={(event) => setDetails(event.target.value)}
                style={textareaStyle}
                placeholder="Describe what happened, what you expected, or what you want Isoprism to support."
              />
            </label>

            <div style={metaStyle}>
              <span>App commit: {appCommit.slice(0, 12)}</span>
              {sourceCommit && <span>Source commit: {sourceCommit.slice(0, 12)}</span>}
            </div>

            {message && (
              <div style={status === "error" ? errorStyle : successStyle}>{message}</div>
            )}

            <div style={actionRowStyle}>
              <button style={secondaryButtonStyle} onClick={close}>
                Cancel
              </button>
              <button
                style={primaryButtonStyle}
                onClick={submit}
                disabled={status === "submitting" || !title.trim() || !details.trim()}
              >
                {status === "submitting" ? "Submitting..." : "Submit"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

const bannerStyle: React.CSSProperties = {
  position: "absolute",
  left: 0,
  right: 0,
  bottom: 0,
  zIndex: 20,
  height: 40,
  background: "#111111",
  color: "#FFFFFF",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: 9,
  fontSize: 13,
  fontWeight: 500,
};

const bannerButtonStyle: React.CSSProperties = {
  border: "none",
  background: "transparent",
  color: "#FFFFFF",
  cursor: "pointer",
  padding: 0,
  textDecoration: "underline",
  textUnderlineOffset: 3,
  fontSize: 13,
  fontWeight: 650,
};

const overlayStyle: React.CSSProperties = {
  position: "fixed",
  inset: 0,
  zIndex: 100,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: "rgba(0, 0, 0, 0.28)",
  padding: 24,
};

const modalStyle: React.CSSProperties = {
  width: "min(520px, 100%)",
  borderRadius: 8,
  border: "1px solid #CFCFCF",
  background: "#FFFFFF",
  padding: 20,
  boxShadow: "0 12px 34px rgba(0,0,0,0.18)",
};

const modalHeaderStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  gap: 16,
  marginBottom: 14,
};

const eyebrowStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 11,
  fontWeight: 700,
  textTransform: "uppercase",
  marginBottom: 5,
};

const titleStyle: React.CSSProperties = {
  margin: 0,
  color: "#111111",
  fontSize: 20,
  lineHeight: 1.2,
  fontWeight: 700,
};

const closeButtonStyle: React.CSSProperties = {
  width: 30,
  height: 30,
  borderRadius: 6,
  border: "1px solid #D4D4D4",
  background: "#F8F8F8",
  color: "#333333",
  cursor: "pointer",
  fontSize: 20,
  lineHeight: 1,
};

const contextStyle: React.CSSProperties = {
  border: "1px solid #E2E2E2",
  borderRadius: 6,
  background: "#F7F7F7",
  color: "#666666",
  fontSize: 12,
  padding: "8px 10px",
  marginBottom: 14,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const labelStyle: React.CSSProperties = {
  display: "grid",
  gap: 6,
  color: "#333333",
  fontSize: 13,
  fontWeight: 650,
  marginBottom: 13,
};

const inputStyle: React.CSSProperties = {
  height: 40,
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  padding: "0 10px",
  color: "#111111",
  fontSize: 14,
  outline: "none",
};

const textareaStyle: React.CSSProperties = {
  minHeight: 120,
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  padding: 10,
  color: "#111111",
  fontSize: 14,
  lineHeight: 1.5,
  outline: "none",
  resize: "vertical",
};

const metaStyle: React.CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: 8,
  color: "#777777",
  fontSize: 11,
  marginBottom: 12,
};

const successStyle: React.CSSProperties = {
  border: "1px solid #BFE2C5",
  background: "#EEF8F0",
  color: "#166534",
  borderRadius: 6,
  padding: "8px 10px",
  fontSize: 13,
  marginBottom: 12,
};

const errorStyle: React.CSSProperties = {
  border: "1px solid #F3B4B4",
  background: "#FFF1F1",
  color: "#991B1B",
  borderRadius: 6,
  padding: "8px 10px",
  fontSize: 13,
  marginBottom: 12,
};

const actionRowStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "flex-end",
  gap: 8,
};

const secondaryButtonStyle: React.CSSProperties = {
  height: 36,
  borderRadius: 6,
  border: "1px solid #D4D4D4",
  background: "#FFFFFF",
  color: "#333333",
  padding: "0 12px",
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 650,
};

const primaryButtonStyle: React.CSSProperties = {
  height: 36,
  borderRadius: 6,
  border: "none",
  background: "#111111",
  color: "#FFFFFF",
  padding: "0 13px",
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 650,
};
