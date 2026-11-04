import { Component, ErrorInfo, ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode | ((error: Error, reset: () => void) => ReactNode);
  name?: string;
  onReset?: () => void;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error(`ErrorBoundary caught error in ${this.props.name || 'component'}:`, error, errorInfo);
  }

  reset = () => {
    this.setState({ hasError: false, error: null });
    this.props.onReset?.();
  };

  render() {
    if (this.state.hasError) {
      if (typeof this.props.fallback === 'function') {
        return this.props.fallback(this.state.error || new Error('Unknown error'), this.reset);
      }
      if (this.props.fallback) {
        return this.props.fallback;
      }
      return (
        <div
          style={{
            padding: '16px',
            background: 'rgba(255, 68, 68, 0.1)',
            border: '1px solid #ff4444',
            borderRadius: '6px',
            margin: '8px',
            color: '#f0e4df',
          }}
        >
          <div style={{ fontWeight: 700, color: '#ff6b6b', marginBottom: '6px' }}>
            {this.props.name || 'Component'} encountered an error.
          </div>
          <div style={{ fontSize: '11px', color: '#8a7a80', marginBottom: '10px', wordBreak: 'break-all' }}>
            {this.state.error?.message || 'Unknown render error'}
          </div>
          <button className="b" onClick={this.reset} style={{ padding: '4px 10px', fontSize: '11px' }}>
            Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
