import type { Metadata } from "next";
import { Geist } from "next/font/google";
import "./globals.css";

const geist = Geist({ subsets: ["latin"] });

export const metadata: Metadata = {
  metadataBase: new URL("https://isoprism.com"),
  title: "Isoprism",
  description: "See what your code changes actually mean.",
  openGraph: {
    title: "Isoprism",
    description: "See what your code changes actually mean.",
    url: "https://isoprism.com",
    siteName: "Isoprism",
    images: [
      {
        url: "/opengraph-image?v=plant-20260520",
        width: 1200,
        height: 630,
        alt: "Isoprism",
      },
    ],
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Isoprism",
    description: "See what your code changes actually mean.",
    images: ["/opengraph-image?v=plant-20260520"],
  },
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "32x32" },
      { url: "/icon.svg", type: "image/svg+xml" },
      { url: "/icon.png", sizes: "512x512", type: "image/png" },
    ],
    apple: [{ url: "/apple-icon.png", sizes: "180x180", type: "image/png" }],
  },
};

// RootLayout wraps every Next route in the shared document shell.
export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className={`${geist.className} antialiased`} style={{ background: "#EBE9E9", color: "#111111" }}>
        {children}
      </body>
    </html>
  );
}
