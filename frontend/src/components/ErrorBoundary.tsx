import { Component, type ReactNode } from "react";

/**
 * Ловит ошибку отрисовки, чтобы она гасила только сломавшийся экран, а не
 * всё окно: без этого любая мелочь оставляет пользователя перед пустотой.
 */
export class ErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | null; stack: string }
> {
  state = { error: null as Error | null, stack: "" };

  static getDerivedStateFromError(error: Error) {
    return { error, stack: error.stack ?? "" };
  }

  componentDidCatch(error: Error, info: { componentStack?: string | null }) {
    this.setState({ stack: (error.stack ?? "") + "\n" + (info.componentStack ?? "") });
  }

  render() {
    if (!this.state.error) return this.props.children;

    return (
      <div className="m-5 rounded-xl border border-red-300 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950">
        <h2 className="text-sm font-semibold text-red-900 dark:text-red-200">Экран не отрисовался</h2>
        <p className="pt-1 text-sm text-red-800 dark:text-red-300">{String(this.state.error)}</p>
        <pre className="selectable mt-3 max-h-64 overflow-auto whitespace-pre-wrap text-xs text-red-700 dark:text-red-400">
          {this.state.stack}
        </pre>
        <button
          className="mt-3 rounded-lg bg-red-600 px-3 py-1.5 text-sm text-white"
          onClick={() => this.setState({ error: null, stack: "" })}
        >
          Попробовать снова
        </button>
      </div>
    );
  }
}
