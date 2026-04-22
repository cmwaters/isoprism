"use client";

interface Props {
  patch: string;
}

export default function DiffBlock({ patch }: Props) {
  const lines = patch.split("\n");

  return (
    <pre style={{
      fontSize: 11,
      fontFamily: "'JetBrains Mono', monospace",
      background: "#1A1A1A",
      borderRadius: 4,
      padding: "8px 10px",
      overflow: "auto",
      margin: 0,
      maxHeight: 300,
    }}>
      {lines.map((line, i) => {
        let color = "#AAAAAA";
        if (line.startsWith("+")) color = "#4ADE80";
        else if (line.startsWith("-")) color = "#F87171";
        else if (line.startsWith("@@")) color = "#818CF8";

        return (
          <span key={i} style={{ color, display: "block" }}>
            {line || " "}
          </span>
        );
      })}
    </pre>
  );
}
