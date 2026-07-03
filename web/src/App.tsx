import { Activity, ClipboardList, Database, ShieldCheck } from 'lucide-react';
import './styles.css';

const surfaces = [
  { title: 'Billing ledger', detail: 'Append-first money movement and usage records.', icon: Database },
  { title: 'Evidence records', detail: 'Receipts, audit events, and task evidence.', icon: ClipboardList },
  { title: 'Tencent reconciliation', detail: 'Compare OPL debits against Tencent cost plus markup.', icon: ShieldCheck },
  { title: 'Kubernetes evidence', detail: 'Read-only runtime snapshots collected through client-go.', icon: Activity }
];

export function App() {
  return (
    <main className="shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Operator console</p>
          <h1>OPL Ledger</h1>
        </div>
        <span className="status">Standalone baseline</span>
      </header>
      <section className="grid" aria-label="Ledger surfaces">
        {surfaces.map((surface) => {
          const Icon = surface.icon;
          return (
            <article className="surface" key={surface.title}>
              <Icon aria-hidden="true" size={22} />
              <h2>{surface.title}</h2>
              <p>{surface.detail}</p>
            </article>
          );
        })}
      </section>
    </main>
  );
}
