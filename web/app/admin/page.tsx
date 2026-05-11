"use client";

import type React from "react";
import { useEffect, useMemo, useState } from "react";
import { API_URL } from "@/lib/api";

type PilotUser = {
  id: string;
  name: string;
  email?: string | null;
  status: string;
  link: string;
  token?: string | null;
  user_id?: string | null;
  accepted_at?: string | null;
  selected_repo_full_name?: string | null;
  trial_starts_at?: string | null;
  trial_ends_at?: string | null;
  review_sent_at?: string | null;
  pilot_languages?: string | null;
  public_repo_url?: string | null;
  issue_count: number;
  feature_count: number;
  questionnaire_submitted_at?: string | null;
};

type PilotForm = {
  id: string;
  pilot_user_id?: string | null;
  form_type: "registration" | "review";
  name?: string | null;
  email?: string | null;
  answers: Record<string, unknown>;
  submitted_at: string;
};

const PASSWORD_STORAGE_KEY = "isoprism_admin_password";

export default function AdminPage() {
  const [password, setPassword] = useState("");
  const [savedPassword, setSavedPassword] = useState("");
  const [tab, setTab] = useState<"users" | "forms">("users");
  const [users, setUsers] = useState<PilotUser[]>([]);
  const [forms, setForms] = useState<PilotForm[]>([]);
  const [expanded, setExpanded] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [manual, setManual] = useState({ name: "", email: "" });

  const activePassword = savedPassword || password;

  useEffect(() => {
    const stored = window.localStorage.getItem(PASSWORD_STORAGE_KEY) ?? "";
    if (stored) {
      setSavedPassword(stored);
      void loadAll(stored);
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
    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }

  async function loadAll(explicitPassword = activePassword) {
    if (!explicitPassword) return;
    setLoading(true);
    setError("");
    try {
      const [userResult, formResult] = await Promise.all([
        adminFetch<{ testers: PilotUser[] }>("/api/v1/admin/pilot/users", undefined, explicitPassword),
        adminFetch<{ forms: PilotForm[] }>("/api/v1/admin/pilot/forms", undefined, explicitPassword),
      ]);
      setUsers(userResult.testers ?? []);
      setForms(formResult.forms ?? []);
      setSavedPassword(explicitPassword);
      window.localStorage.setItem(PASSWORD_STORAGE_KEY, explicitPassword);
    } catch (err) {
      setSavedPassword("");
      window.localStorage.removeItem(PASSWORD_STORAGE_KEY);
      setError(err instanceof Error ? err.message : "Could not load admin data.");
    } finally {
      setLoading(false);
    }
  }

  async function createManualUser(event: React.FormEvent) {
    event.preventDefault();
    if (!manual.name.trim()) return;
    setBusy("create");
    setError("");
    setMessage("");
    try {
      await adminFetch("/api/v1/admin/pilot/users", {
        method: "POST",
        body: JSON.stringify({
          name: manual.name.trim(),
          email: manual.email.trim(),
        }),
      });
      setManual({ name: "", email: "" });
      setMessage("Pilot user added.");
      await loadAll();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not add pilot user.");
    } finally {
      setBusy("");
    }
  }

  async function runUserAction(user: PilotUser, action: "invite" | "review-email" | "delete") {
    const label = `${action}:${user.id}`;
    if (action === "delete" && !window.confirm(`Delete ${user.name}?`)) return;
    setBusy(label);
    setError("");
    setMessage("");
    try {
      if (action === "delete") {
        await adminFetch(`/api/v1/admin/pilot/users/${user.id}`, { method: "DELETE" });
        setMessage("Pilot user deleted.");
      } else {
        const result = await adminFetch<{ link: string }>(`/api/v1/admin/pilot/users/${user.id}/${action}`, { method: "POST" });
        setMessage(`${action === "invite" ? "Invite" : "Review"} email sent. ${result.link}`);
      }
      await loadAll();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed.");
    } finally {
      setBusy("");
    }
  }

  const userForms = useMemo(() => {
    const byUser = new Map<string, PilotForm[]>();
    for (const form of forms) {
      if (!form.pilot_user_id) continue;
      byUser.set(form.pilot_user_id, [...(byUser.get(form.pilot_user_id) ?? []), form]);
    }
    return byUser;
  }, [forms]);

  const registered = users.filter((user) => user.status === "registered" || !user.token);
  const invited = users.filter((user) => user.status !== "registered" && user.token);

  if (!savedPassword) {
    return (
      <Shell>
        <main style={loginStyle}>
          <form style={stackStyle} onSubmit={(event) => { event.preventDefault(); void loadAll(password); }}>
            <input style={inputStyle} type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Admin password" />
            <button style={primaryButtonStyle} disabled={!password || loading}>{loading ? "Checking..." : "Unlock"}</button>
          </form>
          {error && <Notice tone="error">{error}</Notice>}
        </main>
      </Shell>
    );
  }

  return (
    <Shell>
      <main style={mainStyle}>
        <header style={headerStyle}>
          <div>
            <div style={eyebrowStyle}>Admin</div>
            <h1 style={titleStyle}>Pilot</h1>
            <p style={copyStyle}>Review registrations, invite pilot users, and collect end-of-pilot reviews.</p>
          </div>
          <div style={buttonRowStyle}>
            <button
              style={secondaryButtonStyle}
              title="Lock the admin page and forget the saved password."
              onClick={() => { setSavedPassword(""); setPassword(""); window.localStorage.removeItem(PASSWORD_STORAGE_KEY); }}
            >
              Lock
            </button>
          </div>
        </header>

        <div style={tabsStyle}>
          <button style={tab === "users" ? activeTabStyle : tabStyle} onClick={() => setTab("users")}>Pilot Users</button>
          <button style={tab === "forms" ? activeTabStyle : tabStyle} onClick={() => setTab("forms")}>Forms</button>
        </div>

        {message && <Notice tone="success">{message}</Notice>}
        {error && <Notice tone="error">{error}</Notice>}

        {tab === "users" ? (
          <div style={stackStyle}>
            <section style={panelStyle}>
              <h2 style={sectionTitleStyle}>Add user manually</h2>
              <form style={manualGridStyle} onSubmit={createManualUser}>
                <input style={inputStyle} value={manual.name} onChange={(event) => setManual({ ...manual, name: event.target.value })} placeholder="Name" />
                <input style={inputStyle} value={manual.email} onChange={(event) => setManual({ ...manual, email: event.target.value })} placeholder="Email" />
                <button style={primaryButtonStyle} disabled={!manual.name.trim() || busy === "create"}>{busy === "create" ? "Adding..." : "Add user"}</button>
              </form>
            </section>

            <UserSection title="Registered" users={registered} forms={userForms} expanded={expanded} setExpanded={setExpanded} busy={busy} onAction={runUserAction} />
            <UserSection title="Invited" users={invited} forms={userForms} expanded={expanded} setExpanded={setExpanded} busy={busy} onAction={runUserAction} />
          </div>
        ) : (
          <section style={panelStyle}>
            <div style={tableHeaderStyle}>
              <span>Type</span>
              <span>Person</span>
              <span>Submitted</span>
            </div>
            {forms.length === 0 ? <Empty>No forms yet.</Empty> : forms.map((form) => (
              <details key={form.id} style={detailCardStyle}>
                <summary style={summaryStyle}>
                  <span>{titleCase(form.form_type)}</span>
                  <span>{form.name ?? form.email ?? "Anonymous"}</span>
                  <span>{formatDate(form.submitted_at)}</span>
                </summary>
                <pre style={preStyle}>{JSON.stringify(form.answers, null, 2)}</pre>
              </details>
            ))}
          </section>
        )}
      </main>
    </Shell>
  );
}

function UserSection({ title, users, forms, expanded, setExpanded, busy, onAction }: {
  title: string;
  users: PilotUser[];
  forms: Map<string, PilotForm[]>;
  expanded: string;
  setExpanded: (id: string) => void;
  busy: string;
  onAction: (user: PilotUser, action: "invite" | "review-email" | "delete") => void;
}) {
  return (
    <section style={panelStyle}>
      <h2 style={sectionTitleStyle}>{title}</h2>
      {users.length === 0 ? <Empty>No users in this section.</Empty> : users.map((user) => {
        const open = expanded === user.id;
        const linkedForms = forms.get(user.id) ?? [];
        const canSendReviewEmail = Boolean(user.email && user.token && user.user_id);
        const reviewDisabledReason = !user.email
          ? "A review email needs an email address."
          : !user.token
            ? "Send the pilot invite first."
            : !user.user_id
              ? "The pilot user needs to register a GitHub account first."
              : undefined;
        return (
          <div key={user.id} style={userCardStyle}>
            <button style={userButtonStyle} onClick={() => setExpanded(open ? "" : user.id)}>
              <div>
                <strong>{user.name}</strong>
                <div style={mutedStyle}>{user.email ?? "No email"}</div>
              </div>
              <div>{user.selected_repo_full_name ?? user.public_repo_url ?? "No repo yet"}</div>
              <div>{user.issue_count} issues / {user.feature_count} features</div>
              <div>{user.questionnaire_submitted_at ? "Review done" : user.review_sent_at ? "Review sent" : user.status}</div>
            </button>
            {open && (
              <div style={expandedStyle}>
                <Info label="Started" value={user.trial_starts_at ? formatDate(user.trial_starts_at) : "Not started"} />
                <Info label="Invite link" value={user.link || "Not generated"} />
                <Info label="Languages" value={user.pilot_languages ?? "None"} />
                <Info label="Registration form" value={linkedForms.find((form) => form.form_type === "registration")?.id ?? "None"} />
                <div style={buttonRowStyle}>
                  <button style={secondaryButtonStyle} disabled={!user.email || busy === `invite:${user.id}`} onClick={() => onAction(user, "invite")}>{busy === `invite:${user.id}` ? "Sending..." : "Send invite"}</button>
                  <button
                    style={canSendReviewEmail ? secondaryButtonStyle : disabledButtonStyle}
                    title={reviewDisabledReason}
                    disabled={!canSendReviewEmail || busy === `review-email:${user.id}`}
                    onClick={() => onAction(user, "review-email")}
                  >
                    {busy === `review-email:${user.id}` ? "Sending..." : "Send review email"}
                  </button>
                  <button style={dangerButtonStyle} disabled={busy === `delete:${user.id}`} onClick={() => onAction(user, "delete")}>{busy === `delete:${user.id}` ? "Deleting..." : "Delete"}</button>
                </div>
              </div>
            )}
          </div>
        );
      })}
    </section>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return <div><div style={smallLabelStyle}>{label}</div><div style={valueStyle}>{value}</div></div>;
}

function Shell({ children }: { children: React.ReactNode }) {
  return <div style={{ minHeight: "100vh", background: "#EBE9E9", color: "#111111" }}>{children}</div>;
}

function Notice({ children, tone }: { children: React.ReactNode; tone: "success" | "error" }) {
  return <div style={tone === "success" ? successStyle : errorStyle}>{children}</div>;
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div style={emptyStyle}>{children}</div>;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function titleCase(value: string) {
  return value.slice(0, 1).toUpperCase() + value.slice(1);
}

const mainStyle: React.CSSProperties = { width: "min(1180px, 100%)", margin: "0 auto", padding: "42px 24px" };
const loginStyle: React.CSSProperties = { width: "min(420px, 100%)", margin: "0 auto", padding: "120px 24px 48px" };
const stackStyle: React.CSSProperties = { display: "grid", gap: 14 };
const headerStyle: React.CSSProperties = { display: "flex", justifyContent: "space-between", gap: 20, alignItems: "flex-start", marginBottom: 22 };
const eyebrowStyle: React.CSSProperties = { color: "#777", fontSize: 12, fontWeight: 750, textTransform: "uppercase", marginBottom: 6 };
const titleStyle: React.CSSProperties = { margin: 0, fontSize: 30, lineHeight: 1.15 };
const copyStyle: React.CSSProperties = { margin: "7px 0 0", color: "#666", fontSize: 14, lineHeight: 1.5 };
const tabsStyle: React.CSSProperties = { display: "flex", gap: 8, marginBottom: 18 };
const tabStyle: React.CSSProperties = { height: 34, borderWidth: 1, borderStyle: "solid", borderColor: "#D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 12px", cursor: "pointer" };
const activeTabStyle: React.CSSProperties = { ...tabStyle, background: "#111", color: "#FFF", borderColor: "#111" };
const panelStyle: React.CSSProperties = { border: "1px solid #D4D4D4", borderRadius: 8, background: "#FFF", padding: 18 };
const sectionTitleStyle: React.CSSProperties = { margin: "0 0 12px", fontSize: 17 };
const manualGridStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr)) auto", gap: 8 };
const inputStyle: React.CSSProperties = { height: 40, border: "1px solid #D4D4D4", borderRadius: 6, padding: "0 11px", fontSize: 14 };
const primaryButtonStyle: React.CSSProperties = { height: 40, border: 0, borderRadius: 6, background: "#111", color: "#FFF", padding: "0 14px", cursor: "pointer", fontWeight: 700 };
const secondaryButtonStyle: React.CSSProperties = { height: 34, borderWidth: 1, borderStyle: "solid", borderColor: "#D4D4D4", borderRadius: 6, background: "#FFF", padding: "0 11px", cursor: "pointer", fontWeight: 650 };
const disabledButtonStyle: React.CSSProperties = { ...secondaryButtonStyle, background: "#F1F1F1", color: "#888", cursor: "not-allowed" };
const dangerButtonStyle: React.CSSProperties = { ...secondaryButtonStyle, borderColor: "#E7B5B5", background: "#FFF1F1", color: "#8A1F1F" };
const buttonRowStyle: React.CSSProperties = { display: "flex", gap: 8, flexWrap: "wrap" };
const tableHeaderStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "0.6fr 1.2fr 0.8fr", gap: 10, color: "#777", fontSize: 11, fontWeight: 750, textTransform: "uppercase", padding: "0 8px 8px" };
const detailCardStyle: React.CSSProperties = { borderTop: "1px solid #E0E0E0", padding: "10px 8px" };
const summaryStyle: React.CSSProperties = { display: "grid", gridTemplateColumns: "0.6fr 1.2fr 0.8fr", gap: 10, cursor: "pointer", fontSize: 13 };
const preStyle: React.CSSProperties = { margin: "12px 0 0", padding: 12, borderRadius: 6, background: "#F7F7F7", overflow: "auto", fontSize: 12 };
const userCardStyle: React.CSSProperties = { border: "1px solid #E0E0E0", borderRadius: 8, overflow: "hidden", marginTop: 8, background: "#F7F7F7" };
const userButtonStyle: React.CSSProperties = { width: "100%", display: "grid", gridTemplateColumns: "1fr 1fr 0.7fr 0.7fr", gap: 12, border: 0, background: "transparent", padding: 12, cursor: "pointer", textAlign: "left", alignItems: "center" };
const expandedStyle: React.CSSProperties = { borderTop: "1px solid #E0E0E0", padding: 12, display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 12 };
const mutedStyle: React.CSSProperties = { color: "#777", fontSize: 12, marginTop: 2 };
const smallLabelStyle: React.CSSProperties = { color: "#777", fontSize: 11, fontWeight: 750, textTransform: "uppercase", marginBottom: 4 };
const valueStyle: React.CSSProperties = { color: "#222", fontSize: 13, overflowWrap: "anywhere" };
const emptyStyle: React.CSSProperties = { border: "1px dashed #D4D4D4", borderRadius: 8, padding: 24, color: "#777", textAlign: "center", fontSize: 13 };
const successStyle: React.CSSProperties = { border: "1px solid #BFE2C5", borderRadius: 8, background: "#EEF8F0", color: "#225B2D", padding: "10px 12px", fontSize: 13, marginBottom: 14 };
const errorStyle: React.CSSProperties = { border: "1px solid #F3B4B4", borderRadius: 8, background: "#FFF1F1", color: "#991B1B", padding: "10px 12px", fontSize: 13, marginBottom: 14 };
