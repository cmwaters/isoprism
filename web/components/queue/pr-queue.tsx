"use client";

import { useRouter } from "next/navigation";
import { QueuePR } from "@/lib/types";

interface Props {
  prs: QueuePR[];
  repoID: string;
}

export default function PRQueue({ prs, repoID }: Props) {
  const router = useRouter();

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {prs.map((pr, idx) => (
        <PRCard
          key={pr.id}
          pr={pr}
          isTop={idx === 0}
          onClick={() => router.push(`/repos/${repoID}/pr/${pr.id}`)}
        />
      ))}
    </div>
  );
}

function PRCard({ pr, isTop, onClick }: { pr: QueuePR; isTop: boolean; onClick: () => void }) {
  const hoursOpen = Math.floor((Date.now() - new Date(pr.opened_at).getTime()) / 3_600_000);
  const timeLabel = hoursOpen < 24 ? `${hoursOpen}h` : `${Math.floor(hoursOpen / 24)}d`;

  const riskColor = pr.risk_label === "high" ? "#EF4444" : pr.risk_label === "low" ? "#22C55E" : "#F59E0B";

  return (
    <button
      onClick={onClick}
      style={{
        background: "#FFFFFF",
        border: "1px solid #D4D4D4",
        borderLeft: `4px solid ${isTop ? "#6366F1" : "#C7D2FE"}`,
        borderRadius: 8,
        padding: 16,
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        cursor: "pointer",
        textAlign: "left",
        width: "100%",
        transition: "background 150ms ease-out",
      }}
      onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "#F5F5F5"; }}
      onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "#FFFFFF"; }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        {/* Row 1: number + title */}
        <div style={{ display: "flex", alignItems: "baseline", gap: 8, marginBottom: 8 }}>
          <span style={{ color: "#AAAAAA", fontSize: 14, flexShrink: 0 }}>#{pr.number}</span>
          <span style={{ color: "#111111", fontSize: 15, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {pr.title}
          </span>
        </div>

        {/* Row 2: AI summary */}
        {pr.summary && (
          <p style={{ color: "#666666", fontSize: 14, margin: "0 0 10px 0", lineHeight: 1.5 }}>
            {pr.summary}
          </p>
        )}

        {/* Row 3: badges */}
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Badge icon="⏱" label={`${timeLabel} open`} />
          <Badge icon="⬡" label={`${pr.nodes_changed} functions`} />
          {pr.risk_label && (
            <Badge
              icon={<Dot color={riskColor} />}
              label={`${capitalize(pr.risk_label)} risk`}
            />
          )}
        </div>
      </div>

      <span style={{ color: "#AAAAAA", fontSize: 20, marginLeft: 16, flexShrink: 0 }}>›</span>
    </button>
  );
}

function Badge({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span style={{
      display: "inline-flex",
      alignItems: "center",
      gap: 4,
      background: "#F0F0F0",
      border: "1px solid #D4D4D4",
      borderRadius: 4,
      padding: "2px 8px",
      fontSize: 11,
      color: "#666666",
    }}>
      {typeof icon === "string" ? <span>{icon}</span> : icon}
      {label}
    </span>
  );
}

function Dot({ color }: { color: string }) {
  return <span style={{ width: 6, height: 6, borderRadius: "50%", background: color, display: "inline-block" }} />;
}

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
