export type ServerEvent = {
  type: string;
  [key: string]: unknown;
};

type Listener = (event: ServerEvent) => void;

const listeners = new Set<Listener>();

export function emitServerEvent(event: ServerEvent) {
  for (const listener of listeners) {
    listener(event);
  }
}

export function subscribeServerEvent(listener: Listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
