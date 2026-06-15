'use client';
import React, { useState } from 'react';
import { IconCopy, IconCheck } from '../icons';

interface CodeBlockProps {
  code: string;
  lang?: string;
  style?: React.CSSProperties;
}

const CodeBlock: React.FC<CodeBlockProps> = ({ code, lang, style }) => {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };
  return (
    <div style={{ position: 'relative', background: 'var(--c-bg-inset)', border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden', ...style }}>
      {lang && (
        <div style={{ padding: '6px 14px', borderBottom: '1px solid var(--c-border)', fontSize: 11, color: 'var(--c-fg-subtle)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{lang}</span>
          <button onClick={copy} style={{ background: 'none', border: 'none', cursor: 'pointer', color: copied ? 'var(--c-success)' : 'var(--c-fg-subtle)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, fontFamily: 'inherit' }}>
            {copied ? <><IconCheck size={12}/>Copied</> : <><IconCopy size={12}/>Copy</>}
          </button>
        </div>
      )}
      <pre style={{ margin: 0, padding: '14px 16px', fontSize: 12.5, lineHeight: 1.6, color: 'var(--c-fg)', fontFamily: "'IBM Plex Mono', monospace", overflowX: 'auto', whiteSpace: 'pre' }}>
        <code>{code}</code>
      </pre>
    </div>
  );
};

export default CodeBlock;
