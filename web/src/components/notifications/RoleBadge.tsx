import { getRoleColor } from "./messageUtils";

export function RoleBadge({ role }: { role?: string }) {
  if (!role) return null;
  const color = getRoleColor(role);
  return (
    <span
      className={`text-[10px] px-1.5 py-0.5 rounded-md ${color.bg} ${color.text} font-medium`}
    >
      {role}
    </span>
  );
}
