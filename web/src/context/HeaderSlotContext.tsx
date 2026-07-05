/**
 * HeaderSlotContext - Lets each page contribute its own content/actions
 * to the single full-width Header rendered in Layout, without each page
 * having to import Header directly.
 *
 * Usage from a view:
 *   useHeaderSlot({ center: <PresenceLine />, actions: <button>New</button> });
 *
 * Layout renders Header with the latest slot content. When the view
 * unmounts, the slot is cleared automatically.
 *
 * Two contexts back the hook: a state context (consumed only by the
 * Layout's header) and a stable API context (consumed by views). Views
 * therefore never re-render when the slot changes — which also means a
 * view can safely push freshly-created ReactNodes on every render (live
 * search inputs, counts) without update loops.
 */

import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

interface HeaderSlot {
  /** Center slot — per-page summary / breadcrumb / inline status. */
  title?: ReactNode;
  actions?: ReactNode;
  /** When true, the header renders as bare chrome (drawer toggle only).
   *  Pages that render their own self-contained header band (AgentDetail's
   *  HUD bar) use this so no per-view slot content competes with it. */
  hidden?: boolean;
}

interface HeaderSlotApi {
  setSlot: (slot: HeaderSlot) => void;
  clearSlot: () => void;
}

const HeaderSlotStateContext = createContext<HeaderSlot>({});

const HeaderSlotApiContext = createContext<HeaderSlotApi>({
  setSlot: () => undefined,
  clearSlot: () => undefined,
});

export function HeaderSlotProvider({ children }: { children: ReactNode }) {
  const [slot, setSlot] = useState<HeaderSlot>({});
  const api = useMemo<HeaderSlotApi>(
    () => ({ setSlot, clearSlot: () => setSlot({}) }),
    [],
  );
  return (
    <HeaderSlotApiContext.Provider value={api}>
      <HeaderSlotStateContext.Provider value={slot}>
        {children}
      </HeaderSlotStateContext.Provider>
    </HeaderSlotApiContext.Provider>
  );
}

/** Read the current slot — consumed by the Layout's header only. */
export function useHeaderSlotContext() {
  const slot = useContext(HeaderSlotStateContext);
  const api = useContext(HeaderSlotApiContext);
  return { slot, ...api };
}

/**
 * useHeaderSlot - Set the header slot content for this view.
 * Automatically cleared on unmount. Applied on every render (ReactNode
 * identity changes each render anyway); uses a layout effect so
 * controlled inputs living in the header update before paint.
 */
export function useHeaderSlot(slot: HeaderSlot) {
  const { setSlot, clearSlot } = useContext(HeaderSlotApiContext);
  useLayoutEffect(() => {
    setSlot(slot);
    return () => clearSlot();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slot.title, slot.actions, slot.hidden]);
}
