import { Metadata } from "next";

export const metadata: Metadata = {
  title: "Privacy Policy — mycel",
  description:
    "mycel privacy policy — how we collect, use, and protect your data. Learn about your rights, our cookie practices, and how to contact us about data concerns.",
  alternates: {
    canonical: "/privacy",
  },
  robots: {
    index: false,
  },
};

export default function PrivacyLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
