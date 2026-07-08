import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "IntegraDock",
  description: "Turn any REST API into a live, callable AI agent toolset.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-paper text-ink antialiased">
        {children}
      </body>
    </html>
  );
}