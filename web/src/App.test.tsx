import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { App } from './App';

describe('App', () => {
  it('renders Ledger operator surfaces', () => {
    render(<App />);
    expect(screen.getByRole('heading', { name: 'OPL Ledger' })).toBeInTheDocument();
    expect(screen.getByText('Billing ledger')).toBeInTheDocument();
    expect(screen.getByText('Evidence records')).toBeInTheDocument();
    expect(screen.getByText('Tencent reconciliation')).toBeInTheDocument();
    expect(screen.getByText('Kubernetes evidence')).toBeInTheDocument();
  });
});
