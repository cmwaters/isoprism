"use client";

import type React from "react";
import { useEffect, useMemo, useState } from "react";
import { API_URL } from "@/lib/api";

type BetaQuestionnaire = {
  faster_rating?: number | null;
  risk_clarity_rating?: number | null;
  confusing_or_missing?: string | null;
  bugs_hit?: string | null;
  build_next?: string | null;
  would_keep_using?: string | null;
};

type BetaTester = {
  id: string;
  beta_id: string;
  name: string;
  email?: string | null;
  status: string;
  invited_at: string;
  accepted_at?: string | null;
  completed_at?: string | null;
  user_id?: string | null;
  selected_repo_id?: string | null;
  selected_repo_full_name?: string | null;
  trial_starts_at?: string | null;
  trial_ends_at?: string | null;
  questionnaire_submitted_at?: string | null;
  questionnaire?: BetaQuestionnaire | null;
};

type CreateBetaTesterResponse = BetaTester & {
  token: string;
  link: string;
};

const PASSWORD_STORAGE_KEY = "isoprism_admin_password";

export default function BetaAdminPage() {
  const [password, setPassword] = useState("");
  const [savedPassword, setSavedPassword] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [testers, setTesters] = useState<BetaTester[]>([]);
  const [created, setCreated] = useState<CreateBetaTesterResponse | null>(null);
  const [expandedID, setExpandedID] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const activePassword = savedPassword || password;

  useEffect(() => {
    const stored = window.localStorage.getItem(PASSWORD_STORAGE_KEY) ?? "";
    if (stored) {
      setSavedPassword(stored);
      setLoading(true);
      fetch(`${API_URL}/api/v1/admin/beta/testers`, {
        headers: {
          "X-Admin-Password": stored,
        },
      })
        .then(async (response) => {
          if (!response.ok) {
            const text = await response.text();
            throw new Error(text || `Admin API error ${response.status}`);
          }
          return response.json() as Promise<{ testers: BetaTester[] }>;
        })
        .then((result) => setTesters(result.testers ?? []))
        .catch((err) => {
          setSavedPassword("");
          window.localStorage.removeItem(PASSWORD_STORAGE_KEY);
          setError(err instanceof Error ? err.message : "Could not load beta testers.");
        })
        .finally(() => setLoading(false));
    }
  }, []);

  async function adminFetch<T>(path: string, options?: RequestInit, explicitPassword = activePassword): Promise<T> {
    const response = await fetch(`${API_URL}${path}`, {
      ...options,
      headers: {
        "Content-Type": "application/json",
        "X-Admin-Password": explicitPassword,
        ...options?.headers,
      },
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Admin API error ${response.status}`);
    }

    return response.json() as Promise<T>;
  }

  async function loadTesters(explicitPassword = activePassword) {
    if (!explicitPassword) return;
    setLoading(true);
    setError("");

    try {
      const result = await adminFetch<{ testers: BetaTester[] }>("/api/v1/admin/beta/testers", undefined, explicitPassword);
      setTesters(result.testers ?? []);
      setSavedPassword(explicitPassword);
      window.localStorage.setItem(PASSWORD_STORAGE_KEY, explicitPassword);
    } catch (err) {
      setSavedPassword("");
      window.localStorage.removeItem(PASSWORD_STORAGE_KEY);
      setError(err instanceof Error ? err.message : "Could not load beta testers.");
    } finally {
      setLoading(false);
    }
  }

  async function createTester(event: React.FormEvent) {
    event.preventDefault();
    if (!name.trim()) return;

    setCreating(true);
    setCreated(null);
    setError("");

    try {
      const result = await adminFetch<CreateBetaTesterResponse>("/api/v1/admin/beta/testers", {
        method: "POST",
        body: JSON.stringify({ name: name.trim(), email: email.trim() }),
      });
      setCreated(result);
      setName("");
      setEmail("");
      await loadTesters();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create beta tester.");
    } finally {
      setCreating(false);
    }
  }

  const sortedTesters = useMemo(() => testers, [testers]);

  if (!savedPassword) {
    return (
      <AdminShell>
        <section style={loginPanelStyle}>
          <div style={eyebrowStyle}>Admin</div>
          <h1 style={titleStyle}>Beta testers</h1>
          <p style={copyStyle}>Enter the admin password to manage beta invite links and tester progress.</p>

          <form
            style={{ display: "grid", gap: 12, marginTop: 22 }}
            onSubmit={(event) => {
              event.preventDefault();
              void loadTesters(password);
            }}
          >
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Admin password"
              style={inputStyle}
            />
            <button style={primaryButtonStyle} disabled={!password || loading}>
              {loading ? "Checking..." : "Unlock"}
            </button>
          </form>

          {error && <div style={errorStyle}>{error}</div>}
        </section>
      </AdminShell>
    );
  }

  return (
    <AdminShell>
      <main style={mainStyle}>
        <header style={headerStyle}>
          <div>
            <div style={eyebrowStyle}>Admin</div>
            <h1 style={titleStyle}>Beta testers</h1>
            <p style={copyStyle}>Create invite links, monitor setup progress, and review questionnaire answers.</p>
          </div>
          <button
            style={secondaryButtonStyle}
            onClick={() => {
              setSavedPassword("");
              setPassword("");
              window.localStorage.removeItem(PASSWORD_STORAGE_KEY);
            }}
          >
            Lock
          </button>
        </header>

        {error && <div style={errorStyle}>{error}</div>}

        <section style={sectionStyle}>
          <h2 style={sectionTitleStyle}>Create tester</h2>
          <form style={createGridStyle} onSubmit={createTester}>
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Tester name"
              style={inputStyle}
            />
            <input
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="Email or note (optional)"
              style={inputStyle}
            />
            <button style={primaryButtonStyle} disabled={!name.trim() || creating}>
              {creating ? "Generating..." : "Generate link"}
            </button>
          </form>

          {created && (
            <div style={createdBoxStyle}>
              <div style={rowTitleStyle}>{created.name} · {created.beta_id}</div>
              <div style={smallLabelStyle}>Raw token, shown once</div>
              <code style={codeBlockStyle}>{created.token}</code>
              <div style={smallLabelStyle}>Invite link</div>
              <code style={codeBlockStyle}>{created.link}</code>
            </div>
          )}
        </section>

        <section style={sectionStyle}>
          <div style={sectionHeaderStyle}>
            <h2 style={sectionTitleStyle}>Monitor testers</h2>
            <button style={secondaryButtonStyle} onClick={() => void loadTesters()} disabled={loading}>
              {loading ? "Refreshing..." : "Refresh"}
            </button>
          </div>

          <div style={tableStyle}>
            <div style={tableHeaderStyle}>
              <span>Tester</span>
              <span>Status</span>
              <span>Repository</span>
              <span>Questionnaire</span>
            </div>

            {sortedTesters.length === 0 ? (
              <div style={emptyStyle}>No beta testers yet.</div>
            ) : (
              sortedTesters.map((tester) => {
                const expanded = expandedID === tester.id;
                return (
                  <div key={tester.id} style={testerRowStyle}>
                    <button style={rowButtonStyle} onClick={() => setExpandedID(expanded ? null : tester.id)}>
                      <div>
                        <div style={rowTitleStyle}>{tester.name}</div>
                        <div style={rowMetaStyle}>{tester.beta_id}</div>
                      </div>
                      <StatusPill status={tester.status} used={Boolean(tester.accepted_at || tester.user_id)} />
                      <div style={rowMetaStrongStyle}>{tester.selected_repo_full_name ?? "Not set up"}</div>
                      <div style={rowMetaStrongStyle}>{tester.questionnaire_submitted_at ? "Submitted" : "Pending"}</div>
                    </button>

                    {expanded && (
                      <div style={detailStyle}>
                        <Detail label="Email / note" value={tester.email ?? "None"} />
                        <Detail label="Invite used" value={tester.accepted_at ? formatDate(tester.accepted_at) : "No"} />
                        <Detail label="User ID" value={tester.user_id ?? "Not linked"} />
                        <Detail label="Trial" value={trialLabel(tester)} />
                        <Detail label="Selected repo ID" value={tester.selected_repo_id ?? "None"} />
                        {tester.questionnaire ? (
                          <div style={questionnaireStyle}>
                            <Detail label="Faster rating" value={String(tester.questionnaire.faster_rating ?? "None")} />
                            <Detail label="Risk clarity" value={String(tester.questionnaire.risk_clarity_rating ?? "None")} />
                            <Detail label="Confusing or missing" value={tester.questionnaire.confusing_or_missing ?? "None"} />
                            <Detail label="Bugs hit" value={tester.questionnaire.bugs_hit ?? "None"} />
                            <Detail label="Build next" value={tester.questionnaire.build_next ?? "None"} />
                            <Detail label="Would keep using" value={tester.questionnaire.would_keep_using ?? "None"} />
                          </div>
                        ) : (
                          <div style={emptyDetailStyle}>No questionnaire response yet.</div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </section>
      </main>
    </AdminShell>
  );
}

function AdminShell({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ minHeight: "100vh", background: "#EBE9E9", color: "#111111" }}>
      {children}
    </div>
  );
}

function StatusPill({ status, used }: { status: string; used: boolean }) {
  return (
    <span style={{
      ...pillStyle,
      borderColor: used ? "#BFE2C5" : "#D6D6D6",
      background: used ? "#EEF8F0" : "#F3F3F3",
      color: used ? "#166534" : "#555555",
    }}>
      {used ? `${status} · used` : status}
    </span>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div style={detailItemStyle}>
      <div style={smallLabelStyle}>{label}</div>
      <div style={detailValueStyle}>{value}</div>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function trialLabel(tester: BetaTester) {
  if (!tester.trial_starts_at) return "Not started";
  const start = formatDate(tester.trial_starts_at);
  const end = tester.trial_ends_at ? formatDate(tester.trial_ends_at) : "No end date";
  return `${start} to ${end}`;
}

const mainStyle: React.CSSProperties = {
  width: "min(1040px, 100%)",
  margin: "0 auto",
  padding: "48px 24px",
};

const loginPanelStyle: React.CSSProperties = {
  width: "min(420px, 100%)",
  margin: "0 auto",
  padding: "120px 24px 48px",
};

const headerStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: 20,
  marginBottom: 24,
};

const eyebrowStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  fontWeight: 700,
  textTransform: "uppercase",
  marginBottom: 7,
};

const titleStyle: React.CSSProperties = {
  margin: 0,
  color: "#111111",
  fontSize: 30,
  lineHeight: 1.18,
  fontWeight: 750,
};

const copyStyle: React.CSSProperties = {
  color: "#666666",
  fontSize: 14,
  lineHeight: 1.55,
  margin: "7px 0 0",
};

const sectionStyle: React.CSSProperties = {
  border: "1px solid #D4D4D4",
  borderRadius: 8,
  background: "#FFFFFF",
  padding: 20,
  marginBottom: 16,
};

const sectionHeaderStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 12,
  marginBottom: 14,
};

const sectionTitleStyle: React.CSSProperties = {
  margin: "0 0 14px",
  color: "#111111",
  fontSize: 17,
  fontWeight: 700,
};

const createGridStyle: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "minmax(180px, 1fr) minmax(180px, 1fr) auto",
  gap: 8,
  alignItems: "center",
};

const inputStyle: React.CSSProperties = {
  height: 40,
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  background: "#FFFFFF",
  color: "#111111",
  padding: "0 11px",
  fontSize: 14,
  outline: "none",
};

const primaryButtonStyle: React.CSSProperties = {
  height: 40,
  borderRadius: 6,
  border: "none",
  background: "#111111",
  color: "#FFFFFF",
  padding: "0 14px",
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 650,
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

const createdBoxStyle: React.CSSProperties = {
  border: "1px solid #BFE2C5",
  background: "#EEF8F0",
  borderRadius: 8,
  padding: 12,
  marginTop: 14,
  display: "grid",
  gap: 7,
};

const codeBlockStyle: React.CSSProperties = {
  display: "block",
  border: "1px solid #D4D4D4",
  borderRadius: 6,
  background: "#FFFFFF",
  color: "#111111",
  padding: 8,
  overflowWrap: "anywhere",
  fontSize: 12,
};

const tableStyle: React.CSSProperties = {
  display: "grid",
  gap: 8,
};

const tableHeaderStyle: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "1.1fr 0.8fr 1.1fr 0.8fr",
  gap: 10,
  color: "#777777",
  fontSize: 11,
  fontWeight: 700,
  textTransform: "uppercase",
  padding: "0 12px",
};

const testerRowStyle: React.CSSProperties = {
  border: "1px solid #E0E0E0",
  borderRadius: 8,
  background: "#FAFAFA",
  overflow: "hidden",
};

const rowButtonStyle: React.CSSProperties = {
  width: "100%",
  display: "grid",
  gridTemplateColumns: "1.1fr 0.8fr 1.1fr 0.8fr",
  gap: 10,
  alignItems: "center",
  minHeight: 58,
  border: "none",
  background: "transparent",
  padding: "10px 12px",
  cursor: "pointer",
  textAlign: "left",
};

const rowTitleStyle: React.CSSProperties = {
  color: "#111111",
  fontSize: 14,
  fontWeight: 700,
};

const rowMetaStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 12,
  marginTop: 2,
};

const rowMetaStrongStyle: React.CSSProperties = {
  color: "#444444",
  fontSize: 13,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const pillStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "fit-content",
  border: "1px solid #D6D6D6",
  borderRadius: 999,
  padding: "3px 8px",
  fontSize: 12,
  fontWeight: 650,
};

const detailStyle: React.CSSProperties = {
  borderTop: "1px solid #E0E0E0",
  padding: 12,
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
  gap: 10,
};

const questionnaireStyle: React.CSSProperties = {
  gridColumn: "1 / -1",
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
  gap: 10,
  borderTop: "1px solid #E0E0E0",
  paddingTop: 10,
};

const detailItemStyle: React.CSSProperties = {
  minWidth: 0,
};

const smallLabelStyle: React.CSSProperties = {
  color: "#777777",
  fontSize: 11,
  fontWeight: 700,
  textTransform: "uppercase",
  marginBottom: 4,
};

const detailValueStyle: React.CSSProperties = {
  color: "#222222",
  fontSize: 13,
  lineHeight: 1.45,
  overflowWrap: "anywhere",
};

const emptyStyle: React.CSSProperties = {
  border: "1px dashed #D4D4D4",
  borderRadius: 8,
  background: "#FAFAFA",
  padding: "28px 16px",
  color: "#777777",
  fontSize: 13,
  textAlign: "center",
};

const emptyDetailStyle: React.CSSProperties = {
  gridColumn: "1 / -1",
  color: "#777777",
  fontSize: 13,
};

const errorStyle: React.CSSProperties = {
  border: "1px solid #F3B4B4",
  borderRadius: 8,
  background: "#FFF1F1",
  color: "#991B1B",
  padding: "10px 12px",
  fontSize: 13,
  marginTop: 14,
};
