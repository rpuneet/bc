import { AgentIcon } from "./AgentIcon";
import { AgentStatusBadge } from "./AgentStatusBadge";
import { colorFromName } from "./utils/colorFromName";

interface AgentHeroCardProps {
  agent: {
    name: string;
    state: string;
    tool?: string;
    runtime_backend?: string;
    avatar?: {
      variant: string;
      color: number;
    };
  };
}

export function AgentHeroCard({ agent }: AgentHeroCardProps) {
  const variant = (agent.avatar?.variant ?? "monogram") as
    | "geometric"
    | "organic"
    | "monogram";
  const color = agent.avatar?.color ?? colorFromName(agent.name);

  return (
    <div className="flex items-center gap-4">
      <AgentIcon
        name={agent.name}
        variant={variant}
        color={color}
        state={agent.state}
        size={64}
      />
      <div className="flex flex-col gap-1">
        <h2 className="text-lg font-bold text-bc-text">{agent.name}</h2>
        <div className="flex items-center gap-3 text-sm text-bc-muted">
          {agent.runtime_backend && <span>{agent.runtime_backend}</span>}
          {agent.tool && <span>{agent.tool}</span>}
        </div>
        <AgentStatusBadge state={agent.state} />
      </div>
    </div>
  );
}
