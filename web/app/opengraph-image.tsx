import { ImageResponse } from "next/og";

export const alt = "Isoprism";
export const size = {
  width: 1200,
  height: 630,
};
export const contentType = "image/png";

export default function OpenGraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          background: "#EBE9E9",
          color: "#111111",
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          padding: "86px",
          fontFamily: "Inter, Arial, sans-serif",
        }}
      >
        <div style={{ fontSize: 118, lineHeight: 1 }}>🌱</div>
        <div style={{ fontSize: 86, fontWeight: 760, letterSpacing: 0, marginTop: 36 }}>Isoprism</div>
        <div style={{ color: "#5F5F5F", fontSize: 42, lineHeight: 1.25, marginTop: 24, maxWidth: 820 }}>
          See what your code changes actually mean.
        </div>
      </div>
    ),
    size,
  );
}
