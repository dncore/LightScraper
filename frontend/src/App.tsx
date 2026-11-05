import { useCallback, useEffect, useRef, useState } from 'react';
import { GetAppInfo, Ping } from '../wailsjs/go/app/App';
import type { app } from '../wailsjs/go/models';
import { EventsOn } from '../wailsjs/runtime/runtime';
import './App.css';

const HEARTBEAT_EVENT = 'backend:heartbeat';

type ConnectionState = 'connecting' | 'connected' | 'error';

interface Heartbeat {
  Sequence: number;
  SentAt: string;
  Version: string;
}

function isHeartbeat(value: unknown): value is Heartbeat {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const payload = value as Record<string, unknown>;
  return (
    typeof payload.Sequence === 'number' &&
    typeof payload.SentAt === 'string' &&
    typeof payload.Version === 'string'
  );
}

function describeError(cause: unknown): string {
  if (cause instanceof Error) {
    return cause.message;
  }
  if (typeof cause === 'string' && cause.trim() !== '') {
    return cause;
  }
  return 'The backend did not return an error message.';
}

function formatHeartbeatTime(timestamp: string): string {
  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) {
    return timestamp;
  }
  return parsed.toLocaleTimeString();
}

function App() {
  const [info, setInfo] = useState<app.AppInfo | null>(null);
  const [ping, setPing] = useState<app.PingResult | null>(null);
  const [connection, setConnection] = useState<ConnectionState>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [heartbeat, setHeartbeat] = useState<Heartbeat | null>(null);
  const [heartbeatCount, setHeartbeatCount] = useState(0);
  const active = useRef(true);

  const checkBackend = useCallback(async () => {
    setConnection('connecting');
    setError(null);
    try {
      const [nextInfo, nextPing] = await Promise.all([GetAppInfo(), Ping()]);
      if (!active.current) {
        return;
      }
      setInfo(nextInfo);
      setPing(nextPing);
      setConnection('connected');
    } catch (cause) {
      if (!active.current) {
        return;
      }
      setConnection('error');
      setPing(null);
      setError(describeError(cause));
    }
  }, []);

  useEffect(() => {
    active.current = true;

    const unsubscribe = EventsOn(HEARTBEAT_EVENT, (payload: unknown) => {
      if (!isHeartbeat(payload)) {
        setError('Received an invalid background event payload.');
        return;
      }
      setHeartbeat(payload);
      setHeartbeatCount((count) => count + 1);
    });

    void checkBackend();

    return () => {
      active.current = false;
      unsubscribe();
    };
  }, [checkBackend]);

  const version = info?.Version ?? '0.1.0-dev';
  const commit = info?.Commit && info.Commit !== 'unknown' ? info.Commit.slice(0, 7) : 'local';
  const statusLabel =
    connection === 'connected'
      ? 'Backend connected'
      : connection === 'error'
        ? 'Backend unavailable'
        : 'Connecting';

  return (
    <main className="shell">
      <header className="masthead">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            LS
          </span>
          <div>
            <p className="eyebrow">Local-first game library</p>
            <h1>LightScraper</h1>
          </div>
        </div>
        <div className={`status status-${connection}`} role="status" aria-live="polite">
          <span className="status-dot" aria-hidden="true" />
          {statusLabel}
        </div>
      </header>

      <section className="hero" aria-labelledby="hero-heading">
        <div>
          <p className="eyebrow">Desktop foundation</p>
          <h2 id="hero-heading">A small, verifiable starting point.</h2>
          <p className="hero-copy">
            This build validates the Wails bridge, background events, and the SQLite migration path.
            Library scanning and scraping are intentionally not enabled yet.
          </p>
        </div>
        <button
          className="check-button"
          type="button"
          onClick={() => void checkBackend()}
          disabled={connection === 'connecting'}
        >
          {connection === 'connecting' ? 'Checking…' : 'Check backend'}
        </button>
      </section>

      {error ? (
        <div className="error-panel" role="alert">
          <strong>Backend error.</strong> {error}
        </div>
      ) : null}

      <div className="grid">
        <section className="card" aria-labelledby="bridge-heading">
          <div className="card-heading">
            <p className="card-kicker">Go → React</p>
            <h3 id="bridge-heading">Bridge call</h3>
          </div>
          <p className="card-value">{ping?.Message ?? 'Waiting for the backend…'}</p>
          <dl className="details">
            <div>
              <dt>Application</dt>
              <dd>{info?.Name ?? 'LightScraper'}</dd>
            </div>
            <div>
              <dt>Version</dt>
              <dd>{version}</dd>
            </div>
            <div>
              <dt>Commit</dt>
              <dd>{commit}</dd>
            </div>
            <div>
              <dt>Schema</dt>
              <dd>{ping ? `v${ping.SchemaVersion}` : '—'}</dd>
            </div>
            <div>
              <dt>Go runtime</dt>
              <dd>{ping?.GoVersion ?? '—'}</dd>
            </div>
          </dl>
        </section>

        <section className="card" aria-labelledby="events-heading">
          <div className="card-heading">
            <p className="card-kicker">Go background → React</p>
            <h3 id="events-heading">Event stream</h3>
          </div>
          <p className="card-value">
            {heartbeat
              ? `Heartbeat #${heartbeat.Sequence} received`
              : 'Waiting for the first background event…'}
          </p>
          <dl className="details">
            <div>
              <dt>Subscription</dt>
              <dd>backend:heartbeat</dd>
            </div>
            <div>
              <dt>Events seen</dt>
              <dd>{heartbeatCount}</dd>
            </div>
            <div>
              <dt>Last event</dt>
              <dd>{heartbeat ? formatHeartbeatTime(heartbeat.SentAt) : '—'}</dd>
            </div>
            <div>
              <dt>Cleanup</dt>
              <dd>Unsubscribe on unmount</dd>
            </div>
          </dl>
          <p className="event-note">
            The Go loop is stopped by the Wails shutdown hook; the React listener returns the
            generated unsubscribe function from its effect cleanup.
          </p>
        </section>
      </div>

      <footer className="footer">
        <span>LS-T001 foundation</span>
        <span aria-hidden="true">/</span>
        <span>No ROM, provider, or gamelist.xml features enabled</span>
      </footer>
    </main>
  );
}

export default App;
