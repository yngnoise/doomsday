import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "DOOMSDAY — Limited Drop Storefront",
    template: "%s | DOOMSDAY",
  },
  description: "A production-style limited-drop commerce demo with simulated payments and real-time stock.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className="antialiased">
        {children}
      </body>
    </html>
  );
}
