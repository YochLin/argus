import { Component, type ReactNode } from "react";

interface Props {
  message: string;
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

// Wraps only the routed page body (App.tsx), never Sidebar/TopBar — a crash
// in one view (bad API shape, etc.) shouldn't take the whole shell down with
// it; the user should always be able to navigate away via the sidebar. App.tsx
// keys this by route + market, so navigating away (or just flipping the
// US/TW toggle on the same route) remounts a clean instance instead of
// re-showing a stale error from the market/page you left.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return <div className="error-message">{this.props.message}</div>;
    }
    return this.props.children;
  }
}
