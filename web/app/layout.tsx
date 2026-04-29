import type { Metadata } from "next";
import { Geist } from "next/font/google";
import AccountPill from "@/components/account-pill";
import "./globals.css";

const geist = Geist({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "Isoprism",
  description: "See what your code changes actually mean.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className={`${geist.className} antialiased`} style={{ background: "#EBE9E9", color: "#111111" }}>
        <AccountPill />
        {children}
      </body>
    </html>
  );
}
