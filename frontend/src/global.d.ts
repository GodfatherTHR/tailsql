declare global {
  interface Window {
    go?: {
      main?: {
        App?: Record<string, (...args: unknown[]) => Promise<unknown>>;
      };
    };
    runtime?: {
      EventsOn: (eventName: string, callback: (...data: unknown[]) => void) => void;
      EventsOff: (eventName: string) => void;
      EventsEmit: (eventName: string, ...data: unknown[]) => void;
    };
  }
}

export {};
