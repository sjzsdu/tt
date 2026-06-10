import { Component, type ErrorInfo, type ReactNode } from 'react';

type DashboardErrorBoundaryProps = {
  children: ReactNode;
};

type DashboardErrorBoundaryState = {
  error: Error | null;
};

function errorFromUnknown(value: unknown): Error {
  if (value instanceof Error) return value;
  if (typeof value === 'string') return new Error(value);
  try {
    return new Error(JSON.stringify(value));
  } catch {
    return new Error(String(value));
  }
}

export class DashboardErrorBoundary extends Component<DashboardErrorBoundaryProps, DashboardErrorBoundaryState> {
  state: DashboardErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: unknown): DashboardErrorBoundaryState {
    return { error: errorFromUnknown(error) };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Formula dashboard render failed', error, info.componentStack);
  }

  private reset = () => {
    this.setState({ error: null });
  };

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <main className="formula-app empty-screen dashboard-error-screen">
        <div className="dashboard-error-card">
          <p className="dashboard-error-kicker">Formula dashboard crashed</p>
          <h1>页面渲染出错，但运行仍在后台继续</h1>
          <p className="dashboard-error-message">{error.message || String(error)}</p>
          {error.stack && <pre className="dashboard-error-stack">{error.stack}</pre>}
          <div className="dashboard-error-actions">
            <button type="button" onClick={this.reset}>Try again</button>
            <button type="button" onClick={() => window.location.reload()}>Reload page</button>
          </div>
        </div>
      </main>
    );
  }
}
