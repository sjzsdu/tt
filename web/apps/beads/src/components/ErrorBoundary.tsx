import { Component, type ReactNode } from 'react';
import { Button, Result } from 'antd';

type Props = { children: ReactNode };
type State = { error: Error | null };

export class DashboardErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{ padding: 40, textAlign: 'center' }}>
          <Result
            status="error"
            title="Dashboard Error"
            subTitle={this.state.error.message}
            extra={<Button type="primary" onClick={() => window.location.reload()}>Reload</Button>}
          />
        </div>
      );
    }
    return this.props.children;
  }
}
