'use client';
import React, { useState, useEffect } from 'react';
import { IconX, IconCheck, IconSearch, IconPackage, IconGlobe, IconLock, IconAlert } from '../icons';
import Btn from '../ui/Button';
import Input from '../ui/Input';
import Toggle from '../ui/Toggle';
import Select from '../ui/Select';
import CodeBlock from '../ui/CodeBlock';
import Card from '../ui/Card';

import { getUsername } from '../../lib/auth';
import { DOMAIN } from '../../lib/config';
import { rpcFetch } from '../../lib/rpc';

interface ModuleWizardProps {
  onClose: () => void;
  onCreated: (owner: string, name: string) => void;
}

const STEPS = ['Identity', 'Dependencies', 'Configuration', 'Review', 'Published'];

const LINT_PRESETS = [
  { id: 'DEFAULT',  label: 'DEFAULT',  desc: 'Recommended rules for most teams' },
  { id: 'BASIC',    label: 'BASIC',    desc: 'A smaller, more permissive set' },
  { id: 'MINIMAL',  label: 'MINIMAL',  desc: 'Only the most critical checks' },
  { id: 'COMMENTS', label: 'COMMENTS', desc: 'DEFAULT + all comment rules' },
];

const DEPS_SEARCH = [
  { name: `${DOMAIN}/googleapis/googleapis`,           desc: 'Google API protos' },
  { name: `${DOMAIN}/bufbuild/protovalidate`,          desc: 'Validation rules for Protobuf' },
  { name: `${DOMAIN}/grpc/grpc`,                       desc: 'gRPC service definitions' },
  { name: `${DOMAIN}/envoyproxy/protoc-gen-validate`,  desc: 'Legacy PGV annotations' },
];

function cleanError(msg: string): string {
  // Convert raw DB constraint errors to readable messages
  // e.g. "Key (name)=(googleapis/testt) already exists."
  const m = msg.match(/Key \([^)]+\)=\(([^)]+)\) already exists/i);
  if (m) return `Module '${m[1]}' already exists.`;
  return msg;
}

const ModuleWizard: React.FC<ModuleWizardProps> = ({ onClose, onCreated }) => {
  // Derive owner from cookie directly - store may not be hydrated yet
  const currentUser = getUsername() || '';

  const [ownerOptions, setOwnerOptions] = useState<{ value: string; label: string }[]>([]);
  const [owner, setOwner] = useState(currentUser);
  const [step, setStep] = useState(0);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [visibility, setVisibility] = useState<'public' | 'private'>('public');
  const [deps, setDeps] = useState<string[]>([]);
  const [depSearch, setDepSearch] = useState('');
  const [lintPreset, setLintPreset] = useState('DEFAULT');
  const [breaking, setBreaking] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  // Populate owner dropdown: current user + their orgs
  useEffect(() => {
    const user = getUsername() || '';
    const opts: { value: string; label: string }[] = user ? [{ value: user, label: user }] : [];
    setOwner(user);
    setOwnerOptions(opts);

    if (!user) return;
    rpcFetch<{ organizations?: { username: string }[] }>(
      '/hades.api.registry.v1.UserService/GetUser',
      { username: user }
    )
      .then(res => {
        const orgs = (res.organizations || []).map(o => ({ value: o.username, label: o.username }));
        setOwnerOptions([{ value: user, label: user }, ...orgs]);
      })
      .catch(() => {});
  }, []);

  const filteredDeps = DEPS_SEARCH.filter(d => d.name.includes(depSearch) && !deps.includes(d.name));
  const addDep = (n: string) => setDeps(d => [...d, n]);
  const remDep = (n: string) => setDeps(d => d.filter(x => x !== n));

  const bufYaml = `version: v2\nmodules:\n  - path: .\n    name: ${DOMAIN}/${owner}/${name || 'my-module'}\n${deps.length ? `deps:\n${deps.map(d => `  - ${d}`).join('\n')}\n` : ''}lint:\n  use:\n    - ${lintPreset}\nbreaking:\n  use:\n    - ${breaking ? 'FILE' : '# (disabled)'}`;

  const nameValid = name.trim().length >= 2 && /^[a-z][a-z0-9-]*$/.test(name);
  const canNext = step === 0 ? nameValid : true;

  // Check name availability before advancing from step 0
  const handleContinueFromStep0 = async () => {
    setError('');
    setLoading(true);
    try {
      await rpcFetch('/hades.api.registry.v1.ModuleService/GetModule', { owner, name });
      // If it succeeds, module already exists
      setError(`Module '${owner}/${name}' already exists.`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // not_found means name is free - proceed
      if (msg.toLowerCase().includes('not found') || msg.toLowerCase().includes('not_found')) {
        setStep(s => s + 1);
      } else {
        setError(cleanError(msg));
      }
    } finally {
      setLoading(false);
    }
  };

  // Store created module info for the View Module button
  const [createdOwner, setCreatedOwner] = useState('');
  const [createdName, setCreatedName] = useState('');

  const doCreate = async () => {
    if (!name) { setError('Module name is required'); return; }
    setLoading(true);
    setError('');
    try {
      const res = await rpcFetch<{ module?: { name?: string } }>(
        '/hades.api.registry.v1.ModuleService/CreateModuleByName',
        {
          name,
          visibility: visibility === 'private' ? 'E_VISIBILITY_PRIVATE' : 'E_VISIBILITY_PUBLIC',
          description,
        }
      );
      const fullName = res.module?.name || `${owner}/${name}`;
      const parts = fullName.split('/');
      setCreatedOwner(parts[0] || owner);
      setCreatedName(parts.slice(1).join('/') || name);
      setStep(4);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      setError(cleanError(msg));
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 1100, backdropFilter: 'blur(2px)' }}/>
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%,-50%)',
        width: 640, maxHeight: '90vh', background: 'var(--c-bg-default)',
        border: '1px solid var(--c-border)', borderRadius: 12,
        boxShadow: '0 24px 64px rgba(0,0,0,0.5)', zIndex: 1101,
        display: 'flex', flexDirection: 'column', overflow: 'hidden',
      }}>
        {/* Header */}
        <div style={{ padding: '20px 24px', borderBottom: '1px solid var(--c-border)', display: 'flex', alignItems: 'center', gap: 16, flexShrink: 0 }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--c-fg)', marginBottom: 2 }}>New Module</div>

          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex', padding: 4, borderRadius: 4 }}>
            <IconX size={15}/>
          </button>
        </div>

        {/* Progress bar */}
        <div style={{ height: 3, background: 'var(--c-border)', flexShrink: 0 }}>
          <div style={{ height: '100%', background: step === 4 ? 'var(--c-success)' : 'var(--c-accent)', width: `${((step + 1) / STEPS.length) * 100}%`, transition: 'width 0.3s, background 0.3s', borderRadius: '0 2px 2px 0' }}/>
        </div>

        {/* Step dots */}
        <div style={{ display: 'flex', gap: 0, padding: '14px 24px', borderBottom: '1px solid var(--c-border)', flexShrink: 0 }}>
          {STEPS.map((s, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', flex: i < STEPS.length - 1 ? 1 : 0 }}>
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4 }}>
                <div style={{
                  width: 26, height: 26, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 11, fontWeight: 600, flexShrink: 0,
                  background: i < step ? 'var(--c-success)' : i === step ? 'var(--c-accent)' : 'var(--c-bg-subtle)',
                  color: i <= step ? '#fff' : 'var(--c-fg-subtle)',
                  border: `2px solid ${i === step ? 'var(--c-accent)' : i < step ? 'var(--c-success)' : 'var(--c-border)'}`,
                }}>
                  {i < step ? <IconCheck size={12}/> : i + 1}
                </div>
                <div style={{ fontSize: 10, color: i === step ? 'var(--c-accent)' : 'var(--c-fg-subtle)', fontWeight: i === step ? 600 : 400, whiteSpace: 'nowrap' }}>{s}</div>
              </div>
              {i < STEPS.length - 1 && (
                <div style={{ flex: 1, height: 2, background: i < step ? 'var(--c-success)' : 'var(--c-border)', margin: '0 6px', marginBottom: 16, transition: 'background 0.3s' }}/>
              )}
            </div>
          ))}
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 24 }}>
          {error && (
            <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8 }}>
              <IconAlert size={14}/>{error}
            </div>
          )}

          {/* Step 0: Identity */}
          {step === 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '180px 1fr', gap: 12 }}>
                <div>
                  <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Owner</label>
                  <Select
                    value={owner}
                    onChange={setOwner}
                    options={ownerOptions.length > 0 ? ownerOptions : [{ value: '', label: '(loading…)' }]}
                    style={{ width: '100%' }}
                  />
                </div>
                <div>
                  <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>
                    Module name <span style={{ color: 'var(--c-danger)' }}>*</span>
                  </label>
                  <Input
                    value={name}
                    onChange={setName}
                    placeholder="e.g. notifications"
                    prefix={<span style={{ fontSize: 12, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg-subtle)' }}>/</span>}
                  />
                  {name && !/^[a-z][a-z0-9-]*$/.test(name) && (
                    <div style={{ fontSize: 11, color: 'var(--c-danger)', marginTop: 4 }}>Lowercase letters, numbers and hyphens only</div>
                  )}
                </div>
              </div>
              {name && /^[a-z][a-z0-9-]*$/.test(name) && (
                <div style={{ padding: '10px 14px', background: 'var(--c-bg-inset)', borderRadius: 6, border: '1px solid var(--c-border)', fontSize: 13 }}>
                  <span style={{ color: 'var(--c-fg-subtle)' }}>{DOMAIN}/</span>
                  <span style={{ color: 'var(--c-accent)', fontFamily: "'IBM Plex Mono', monospace", fontWeight: 500 }}>{owner}/{name}</span>
                </div>
              )}
              <div>
                <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Description</label>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="What does this module define?"
                  rows={3}
                  style={{ width: '100%', background: 'var(--c-bg-inset)', border: '1px solid var(--c-border)', borderRadius: 6, color: 'var(--c-fg)', fontSize: 13, fontFamily: 'inherit', padding: '8px 12px', resize: 'vertical', boxSizing: 'border-box', outline: 'none' }}
                />
              </div>
              <div>
                <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 8 }}>Visibility</label>
                <div style={{ display: 'flex', gap: 10 }}>
                  {(['public', 'private'] as const).map(v => (
                    <div key={v} onClick={() => setVisibility(v)} style={{
                      flex: 1, padding: '12px 16px', borderRadius: 8,
                      border: `2px solid ${visibility === v ? 'var(--c-accent)' : 'var(--c-border)'}`,
                      background: visibility === v ? 'var(--c-accent-bg)' : 'var(--c-bg-inset)',
                      cursor: 'pointer', transition: 'all 0.15s',
                    }}>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 4, fontSize: 13, fontWeight: 600, color: visibility === v ? 'var(--c-accent)' : 'var(--c-fg)' }}>
                        {v === 'public' ? <IconGlobe size={14}/> : <IconLock size={14}/>}
                        {v.charAt(0).toUpperCase() + v.slice(1)}
                      </div>
                      <div style={{ fontSize: 12, color: 'var(--c-fg-subtle)' }}>
                        {v === 'public' ? 'Anyone can view and depend on this module' : 'Only org members can access'}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* Step 1: Dependencies */}
          {step === 1 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.6 }}>
                Add {DOMAIN} modules your protos import. These will be added to your <code style={{ fontFamily: "'IBM Plex Mono', monospace" }}>buf.yaml</code>.
              </div>
              <Input value={depSearch} onChange={setDepSearch} placeholder={`Search ${DOMAIN} registry…`} prefix={<IconSearch size={14}/>}/>
              {depSearch && filteredDeps.length > 0 && (
                <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden' }}>
                  {filteredDeps.map((d, i) => (
                    <div key={d.name} onClick={() => addDep(d.name)}
                      style={{ padding: '11px 16px', display: 'flex', gap: 12, alignItems: 'center', cursor: 'pointer', borderBottom: i < filteredDeps.length - 1 ? '1px solid var(--c-border-muted)' : 'none' }}
                      onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'}
                      onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
                    >
                      <code style={{ fontSize: 12, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)', flex: 1 }}>{d.name}</code>
                      <span style={{ fontSize: 12, color: 'var(--c-fg-subtle)' }}>{d.desc}</span>
                      <Btn size="sm" variant="ghost">Add</Btn>
                    </div>
                  ))}
                </div>
              )}
              {deps.length > 0 ? (
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.6, marginBottom: 8 }}>Added ({deps.length})</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    {deps.map(d => (
                      <div key={d} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 14px', background: 'var(--c-bg-inset)', borderRadius: 6, border: '1px solid var(--c-border)' }}>
                        <IconPackage size={13} style={{ color: 'var(--c-accent)' }}/>
                        <code style={{ fontSize: 12, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg)', flex: 1 }}>{d}</code>
                        <button onClick={() => remDep(d)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-subtle)', display: 'flex', padding: 2 }}><IconX size={12}/></button>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <div style={{ padding: 24, textAlign: 'center', color: 'var(--c-fg-subtle)', fontSize: 13, border: '2px dashed var(--c-border)', borderRadius: 8 }}>
                  No dependencies yet - you can add them later
                </div>
              )}
            </div>
          )}

          {/* Step 2: Configuration */}
          {step === 2 && (
            <div style={{ display: 'flex', gap: 20 }}>
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div>
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 10 }}>Lint Rules</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    {LINT_PRESETS.map(p => (
                      <div key={p.id} onClick={() => setLintPreset(p.id)} style={{
                        padding: '10px 14px', borderRadius: 7,
                        border: `1px solid ${lintPreset === p.id ? 'var(--c-accent)' : 'var(--c-border)'}`,
                        background: lintPreset === p.id ? 'var(--c-accent-bg)' : 'transparent',
                        cursor: 'pointer', transition: 'all 0.1s',
                      }}>
                        <div style={{ fontSize: 13, fontWeight: 500, color: lintPreset === p.id ? 'var(--c-accent)' : 'var(--c-fg)', fontFamily: "'IBM Plex Mono', monospace" }}>{p.label}</div>
                        <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', marginTop: 2 }}>{p.desc}</div>
                      </div>
                    ))}
                  </div>
                </div>
                <div>
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 10 }}>Breaking Change Detection</div>
                  <Card style={{ padding: '14px 16px', display: 'flex', gap: 12, alignItems: 'center' }}>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--c-fg)' }}>Enable file-level detection</div>
                      <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', marginTop: 2 }}>Block pushes that break existing consumers</div>
                    </div>
                    <Toggle checked={breaking} onChange={setBreaking}/>
                  </Card>
                </div>
              </div>
              <div style={{ width: 240, flexShrink: 0 }}>
                <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--c-fg-muted)', marginBottom: 8 }}>buf.yaml preview</div>
                <CodeBlock code={bufYaml} lang="yaml" style={{ fontSize: 11 }}/>
              </div>
            </div>
          )}

          {/* Step 3: Review */}
          {step === 3 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Card style={{ padding: '18px 20px' }}>
                <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.6, marginBottom: 12 }}>Module Summary</div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  {([
                    ['Name', <span style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)' }}>{owner}/{name}</span>],
                    ['Visibility', visibility],
                    ['Dependencies', deps.length || 'None'],
                    ['Lint Rules', lintPreset],
                    ['Breaking', breaking ? 'FILE level' : 'Disabled'],
                    ['Owner', owner],
                  ] as [string, React.ReactNode][]).map(([k, v]) => (
                    <div key={k}>
                      <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', marginBottom: 3 }}>{k}</div>
                      <div style={{ fontSize: 13, color: 'var(--c-fg)' }}>{v}</div>
                    </div>
                  ))}
                </div>
              </Card>
              <CodeBlock code={bufYaml} lang="yaml"/>
              <div style={{ padding: '12px 16px', background: 'var(--c-warning-bg)', border: '1px solid var(--c-warning)', borderRadius: 8, fontSize: 13, color: 'var(--c-warning)', display: 'flex', gap: 10 }}>
                <IconAlert size={14} style={{ marginTop: 1, flexShrink: 0 }}/> Publishing is irreversible. The module name and owner cannot be changed after creation.
              </div>
            </div>
          )}

          {/* Step 4: Success */}
          {step === 4 && (
            <div style={{ textAlign: 'center', padding: '24px 0' }}>
              <div style={{ width: 64, height: 64, borderRadius: '50%', background: 'var(--c-success-bg)', border: '2px solid var(--c-success)', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 20px' }}>
                <IconCheck size={28} style={{ color: 'var(--c-success)' }}/>
              </div>
              <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--c-fg)', marginBottom: 6, letterSpacing: -0.3 }}>Module published!</div>
              <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 24 }}>
                <code style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)' }}>{createdOwner}/{createdName}</code> is now live on the registry.
              </div>
              <CodeBlock code={`buf push --tag v0.1.0`} lang="bash"/>
              <div style={{ marginTop: 20, display: 'flex', gap: 10, justifyContent: 'center' }}>
                <Btn variant="primary" onClick={() => onCreated(createdOwner, createdName)}>View Module</Btn>
                <Btn onClick={onClose}>Close</Btn>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        {step < 4 && (
          <div style={{ padding: '16px 24px', borderTop: '1px solid var(--c-border)', display: 'flex', justifyContent: 'space-between', flexShrink: 0, background: 'var(--c-bg-overlay)' }}>
            <Btn onClick={() => { if (step > 0) setStep(s => s - 1); else onClose(); }} variant="ghost">
              {step === 0 ? 'Cancel' : '← Back'}
            </Btn>
            <div style={{ display: 'flex', gap: 10 }}>
              {step < 3 && <Btn variant="ghost" onClick={() => setStep(s => s + 1)}>Skip</Btn>}
              {step < 3 && (
                <Btn
                  variant="primary"
                  disabled={!canNext || loading}
                  onClick={step === 0 ? handleContinueFromStep0 : () => setStep(s => s + 1)}
                >
                  {loading && step === 0 ? 'Checking…' : 'Continue →'}
                </Btn>
              )}
              {step === 3 && <Btn variant="primary" onClick={doCreate} disabled={loading}>{loading ? 'Publishing…' : 'Publish Module'}</Btn>}
            </div>
          </div>
        )}
      </div>
    </>
  );
};

export default ModuleWizard;
