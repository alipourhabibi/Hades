import React from 'react';

interface Column<T> {
  key: keyof T & string;
  label: string;
  width?: string;
  render?: (value: T[keyof T], row: T) => React.ReactNode;
}

interface TableProps<T extends object> {
  columns: Column<T>[];
  rows: T[];
  onRowClick?: (row: T) => void;
}

function Table<T extends object>({ columns, rows, onRowClick }: TableProps<T>) {
  return (
    <div style={{ overflow: 'auto', border: '1px solid var(--c-border)', borderRadius: 8 }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 400 }}>
        <thead>
          <tr style={{ borderBottom: '1px solid var(--c-border)', background: 'var(--c-bg-overlay)' }}>
            {columns.map(col => (
              <th key={col.key} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 11, fontWeight: 600, color: 'var(--c-fg-muted)', letterSpacing: 0.5, textTransform: 'uppercase', whiteSpace: 'nowrap', width: col.width }}>{col.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} onClick={() => onRowClick && onRowClick(row)}
              style={{ borderBottom: i < rows.length - 1 ? '1px solid var(--c-border-muted)' : 'none', cursor: onRowClick ? 'pointer' : 'default' }}
              onMouseEnter={e => { if (onRowClick) (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'; }}
              onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = 'transparent'; }}>
              {columns.map(col => (
                <td key={col.key} style={{ padding: '12px 16px', fontSize: 13, color: 'var(--c-fg)', verticalAlign: 'middle' }}>
                  {col.render ? col.render(row[col.key], row) : String(row[col.key] ?? '')}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default Table;
