import { MethodContent } from "./MethodContent";
import { absoluteUrl } from "../../lib/site";

export const metadata = {
  title: "The mycel method — how it works, and why",
  description:
    "How a team of AI agents actually works: from first launch to a reviewed diff in five steps, and the six convictions underneath — isolation, communication, visibility, cost, persistence, openness.",
  alternates: {
    canonical: "/method",
  },
  openGraph: {
    type: "article",
    url: absoluteUrl("/method"),
    title: "The mycel method — how it works, and why",
    description:
      "From first launch to a reviewed diff in five steps, and the six convictions underneath.",
  },
};

export default function MethodPage() {
  return <MethodContent />;
}
