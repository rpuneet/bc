/** WebSocket event types from the daemon */
export type WSEventType =
  | "agent.created"
  | "agent.started"
  | "agent.stopped"
  | "agent.deleted"
  | "agent.state_changed"
  | "agent.output"
  | "agent.hook"
  | "channel.message"
  | "cost.updated"
  | "cost.budget_alert"
  | "gateway.message"
  | "gateway.connected"
  | "gateway.disconnected"
  | "gateway.delivery"
  | "connected";

export interface WSEvent {
  type: WSEventType;
  data: Record<string, unknown>;
  timestamp: string;
}
