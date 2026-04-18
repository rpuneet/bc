import { getRoleColor } from "./messageUtils";

export function AgentAvatar({
  name,
  role,
  size = "md",
}: {
  name: string;
  role?: string;
  size?: "sm" | "md";
}) {
  const color = getRoleColor(role);
  const sizeClass = size === "sm" ? "w-6 h-6 text-[10px]" : "w-8 h-8 text-xs";

  return (
    <div
      className={`${sizeClass} ${color.bg} ${color.text} rounded-full flex items-center justify-center font-semibold shrink-0 uppercase`}
      aria-label={name}
    >
      {name.charAt(0)}
    </div>
  );
}

export function RoleBadge({ role }: { role?: string }) {
  if (!role) return null;
  const color = getRoleColor(role);
  return (
    <span
      className={`text-[10px] px-1.5 py-0.5 rounded ${color.bg} ${color.text} font-medium`}
    >
      {role}
    </span>
  );
}
