/**
 * HeaderSlotContext - Lets each page contribute its own title/actions
 * to the single Header rendered in Layout, without each page having
 * to import Header directly.
 *
 * Usage from a view:
 *   useHeaderSlot({ title: <>Agents</>, actions: <button>New</button> });
 *
 * Layout renders Header with the latest slot content. When the view
 * unmounts, the slot is cleared automatically.
 */

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

interface HeaderSlot {
  title?: ReactNode;
  actions?: ReactNode;
}

interface HeaderSlotContextValue {
  slot: HeaderSlot;
  setSlot: (slot: HeaderSlot) => void;
  clearSlot: () => void;
}

const HeaderSlotContext = createContext<HeaderSlotContextValue>({
  slot: {},
  setSlot: () => undefined,
  clearSlot: () => undefined,
});

export function HeaderSlotProvider({ children }: { children: ReactNode }) {
  const [slot, setSlot] = useState<HeaderSlot>({});
  const value = useMemo<HeaderSlotContextValue>(
    () => ({ slot, setSlot, clearSlot: () => setSlot({}) }),
    [slot],
  );
  return <HeaderSlotContext.Provider value={value}>{children}</HeaderSlotContext.Provider>;
}

export function useHeaderSlotContext() {
  return useContext(HeaderSlotContext);
}

/**
 * useHeaderSlot - Set the header slot content for this view.
 * Automatically cleared on unmount. Re-runs whenever the slot object
 * identity changes, so callers should memo their ReactNodes if they
 * depend on state.
 */
export function useHeaderSlot(slot: HeaderSlot) {
  const { setSlot, clearSlot } = useHeaderSlotContext();
  // Serialize to detect meaningful change — ReactNode refs change every render,
  // so just apply on every render and clear on unmount.
  useEffect(() => {
    setSlot(slot);
    return () => clearSlot();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slot.title, slot.actions]);
}
