'use client';
import React, { useState, useMemo } from 'react';
import hljs from 'highlight.js/lib/core';
import { marked } from 'marked';

// Register only the grammars we actually need to keep the bundle small.
import protobuf    from 'highlight.js/lib/languages/protobuf';
import go          from 'highlight.js/lib/languages/go';
import typescript  from 'highlight.js/lib/languages/typescript';
import javascript  from 'highlight.js/lib/languages/javascript';
import python      from 'highlight.js/lib/languages/python';
import yaml        from 'highlight.js/lib/languages/yaml';
import json        from 'highlight.js/lib/languages/json';
import bash        from 'highlight.js/lib/languages/bash';
import xml         from 'highlight.js/lib/languages/xml';
import css         from 'highlight.js/lib/languages/css';
import sql         from 'highlight.js/lib/languages/sql';
import markdown    from 'highlight.js/lib/languages/markdown';

hljs.registerLanguage('protobuf',   protobuf);
hljs.registerLanguage('go',         go);
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('python',     python);
hljs.registerLanguage('yaml',       yaml);
hljs.registerLanguage('json',       json);
hljs.registerLanguage('bash',       bash);
hljs.registerLanguage('xml',        xml);
hljs.registerLanguage('css',        css);
hljs.registerLanguage('sql',        sql);
hljs.registerLanguage('markdown',   markdown);

// ── Language detection ────────────────────────────────────────────────────

const EXT_LANG: Record<string, string> = {
  proto:      'protobuf',
  go:         'go',
  ts:         'typescript',
  tsx:        'typescript',
  js:         'javascript',
  jsx:        'javascript',
  mjs:        'javascript',
  cjs:        'javascript',
  py:         'python',
  yaml:       'yaml',
  yml:        'yaml',
  json:       'json',
  sh:         'bash',
  bash:       'bash',
  zsh:        'bash',
  xml:        'xml',
  html:       'xml',
  htm:        'xml',
  svg:        'xml',
  css:        'css',
  scss:       'css',
  sql:        'sql',
  md:         'markdown',
  markdown:   'markdown',
};

function detectLanguage(filename: string): string | null {
  const lower = filename.toLowerCase();
  const ext   = lower.split('.').pop() ?? '';
  return EXT_LANG[ext] ?? null;
}

// Filenames that should be rendered as Markdown (not highlighted as code).
const README_NAMES = new Set(['readme', 'readme.md', 'readme.mdx', 'readme.markdown', 'changelog.md', 'contributing.md']);

function isMarkdownFile(filename: string): boolean {
  return README_NAMES.has(filename.toLowerCase()) ||
         filename.toLowerCase().endsWith('.md') ||
         filename.toLowerCase().endsWith('.markdown');
}

// ── Highlight.js theme (GitHub Dark style, CSS-in-JS) ────────────────────
// These variables map highlight.js token classes to colours.
const HLJS_THEME = `
.hljs { background: transparent; color: var(--c-fg); }
.hljs-comment, .hljs-quote          { color: #8b949e; font-style: italic; }
.hljs-keyword, .hljs-selector-tag,
.hljs-built_in, .hljs-name,
.hljs-tag .hljs-name                { color: #ff7b72; }
.hljs-string, .hljs-attr,
.hljs-template-variable,
.hljs-template-tag                  { color: #a5d6ff; }
.hljs-number, .hljs-literal,
.hljs-variable, .hljs-template-variable,
.hljs-tag .hljs-attr                { color: #79c0ff; }
.hljs-type, .hljs-class .hljs-title,
.hljs-title.class_                  { color: #ffa657; }
.hljs-title, .hljs-title.function_  { color: #d2a8ff; }
.hljs-section                       { color: #1f6feb; }
.hljs-addition                      { color: #aff5b4; background: #033a16; }
.hljs-deletion                      { color: #ffdcd7; background: #67060c; }
.hljs-meta                          { color: #e3b341; }
.hljs-link                          { color: #a5d6ff; text-decoration: underline; }
.hljs-emphasis                      { font-style: italic; }
.hljs-strong                        { font-weight: bold; }
/* Protobuf-specific */
.hljs-symbol                        { color: #79c0ff; }
.hljs-bullet                        { color: #ffa657; }
`;

// ── Markdown stylesheet ───────────────────────────────────────────────────
const MD_THEME = `
.md-body { font-size: 13.5px; line-height: 1.7; color: var(--c-fg); }
.md-body h1,.md-body h2,.md-body h3,.md-body h4 {
  margin: 1.4em 0 0.5em; font-weight: 600; color: var(--c-fg);
  padding-bottom: 0.25em; border-bottom: 1px solid var(--c-border-muted);
}
.md-body h1 { font-size: 1.5em; }
.md-body h2 { font-size: 1.25em; }
.md-body h3 { font-size: 1.05em; border-bottom: none; }
.md-body p  { margin: 0.75em 0; }
.md-body a  { color: var(--c-accent); text-decoration: none; }
.md-body a:hover { text-decoration: underline; }
.md-body code {
  font-family: 'IBM Plex Mono', monospace; font-size: 0.85em;
  background: var(--c-bg-overlay); padding: 1px 5px; border-radius: 4px;
}
.md-body pre {
  background: var(--c-bg-inset); border: 1px solid var(--c-border);
  border-radius: 6px; padding: 14px 16px; overflow-x: auto; margin: 1em 0;
}
.md-body pre code { background: none; padding: 0; font-size: 0.875em; }
.md-body blockquote {
  margin: 0.75em 0; padding: 0.5em 1em;
  border-left: 3px solid var(--c-border); color: var(--c-fg-muted);
}
.md-body ul,.md-body ol { padding-left: 1.5em; margin: 0.5em 0; }
.md-body li  { margin: 0.25em 0; }
.md-body hr  { border: none; border-top: 1px solid var(--c-border-muted); margin: 1.5em 0; }
.md-body table { border-collapse: collapse; width: 100%; margin: 1em 0; }
.md-body th, .md-body td {
  border: 1px solid var(--c-border); padding: 6px 12px; text-align: left;
}
.md-body th { background: var(--c-bg-subtle); font-weight: 600; }
.md-body img { max-width: 100%; border-radius: 4px; }
`;

// ── FileViewer ────────────────────────────────────────────────────────────

export interface FileViewerProps {
  /** Base filename (e.g. "service.proto") - used for language detection. */
  filename: string;
  /** Raw file content as a UTF-8 string. */
  content: string;
  /** Short git OID (7 chars) shown in the header. */
  oid?: string;
  /** Called when the user clicks the back / breadcrumb area. */
  onBack?: () => void;
}

const FileViewer: React.FC<FileViewerProps> = ({ filename, content, oid }) => {
  const [copied, setCopied] = useState(false);

  const isMarkdown = isMarkdownFile(filename);
  const lang       = detectLanguage(filename);

  const copy = () => {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  // Highlighted HTML - computed once per content/language change.
  const highlightedHtml = useMemo<string | null>(() => {
    if (isMarkdown) return null; // handled separately
    if (!lang) return null;      // fall back to plain text
    try {
      return hljs.highlight(content, { language: lang }).value;
    } catch {
      return null;
    }
  }, [content, lang, isMarkdown]);

  // Rendered markdown HTML.
  const markdownHtml = useMemo<string | null>(() => {
    if (!isMarkdown) return null;
    try {
      const raw = marked.parse(content, { async: false }) as string;
      return raw;
    } catch {
      return null;
    }
  }, [content, isMarkdown]);

  // Line count for the gutter.
  const lines = content.split('\n');

  const langLabel = isMarkdown ? 'markdown' : (lang ?? 'text');

  return (
    <>
      {/* Inject theme styles once */}
      <style>{HLJS_THEME}</style>
      <style>{MD_THEME}</style>

      <div style={{
        border: '1px solid var(--c-border)',
        borderRadius: 8,
        overflow: 'hidden',
        background: 'var(--c-bg-default)',
      }}>
        {/* ── Header bar ──────────────────────────────────────────────── */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '9px 16px',
          borderBottom: '1px solid var(--c-border-muted)',
          background: 'var(--c-bg-subtle)',
        }}>
          <span style={{
            flex: 1, fontSize: 13, fontWeight: 600,
            fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg)',
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          }}>
            {filename}
          </span>

          {/* Language badge */}
          <span style={{
            fontSize: 10, fontWeight: 500, textTransform: 'uppercase',
            letterSpacing: '0.05em', padding: '2px 7px', borderRadius: 4,
            background: 'var(--c-bg-overlay)', color: 'var(--c-fg-muted)',
            flexShrink: 0,
          }}>
            {langLabel}
          </span>

          {/* Short OID */}
          {oid && (
            <code style={{
              fontSize: 11, fontFamily: "'IBM Plex Mono', monospace",
              color: 'var(--c-fg-subtle)', background: 'var(--c-bg-overlay)',
              padding: '2px 7px', borderRadius: 4, flexShrink: 0,
            }}>
              {oid.slice(0, 7)}
            </code>
          )}

          {/* Line count */}
          <span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', flexShrink: 0 }}>
            {lines.length} {lines.length === 1 ? 'line' : 'lines'}
          </span>

          {/* Copy button */}
          <button
            onClick={copy}
            style={{
              background: 'none', border: '1px solid var(--c-border)',
              borderRadius: 5, cursor: 'pointer', padding: '3px 9px',
              fontSize: 11, color: copied ? 'var(--c-success)' : 'var(--c-fg-muted)',
              fontFamily: 'inherit', flexShrink: 0,
              transition: 'color 0.15s',
            }}
          >
            {copied ? 'Copied!' : 'Copy'}
          </button>
        </div>

        {/* ── Markdown view ────────────────────────────────────────────── */}
        {isMarkdown && markdownHtml !== null && (
          <div
            className="md-body"
            style={{ padding: '20px 28px', overflowX: 'auto' }}
            dangerouslySetInnerHTML={{ __html: markdownHtml }}
          />
        )}

        {/* ── Syntax-highlighted code view ─────────────────────────────── */}
        {!isMarkdown && (
          <div style={{
            display: 'flex',
            overflowX: 'auto',
            background: 'var(--c-bg-inset)',
          }}>
            {/* Line-number gutter */}
            <div aria-hidden style={{
              padding: '14px 0',
              minWidth: 44,
              textAlign: 'right',
              userSelect: 'none',
              borderRight: '1px solid var(--c-border-muted)',
              background: 'var(--c-bg-subtle)',
              flexShrink: 0,
            }}>
              {lines.map((_, i) => (
                <div key={i} style={{
                  padding: '0 12px',
                  fontSize: 11.5,
                  lineHeight: 1.65,
                  fontFamily: "'IBM Plex Mono', monospace",
                  color: 'var(--c-fg-subtle)',
                }}>
                  {i + 1}
                </div>
              ))}
            </div>

            {/* Code content */}
            <pre style={{
              margin: 0, flex: 1,
              padding: '14px 20px',
              fontSize: 12.5, lineHeight: 1.65,
              fontFamily: "'IBM Plex Mono', monospace",
              overflowX: 'auto', whiteSpace: 'pre',
              tabSize: 2,
            }}>
              {highlightedHtml !== null ? (
                <code dangerouslySetInnerHTML={{ __html: highlightedHtml }} />
              ) : (
                <code style={{ color: 'var(--c-fg)' }}>{content}</code>
              )}
            </pre>
          </div>
        )}
      </div>
    </>
  );
};

export default FileViewer;
