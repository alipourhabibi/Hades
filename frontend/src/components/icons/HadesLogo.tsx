import React from 'react';

interface HadesLogoProps {
  /** Rendered size of the icon in px (the viewBox is always 120×120) */
  size?: number;
  /** 'dark' uses the dark-background gradient palette; 'light' uses the light palette */
  theme?: 'dark' | 'light';
  /** Unique id suffix - required when multiple instances appear on the same page */
  id?: string;
}

/**
 * The Hades icon mark - hexagon frame + shield outline + "H" letterform.
 * Use <HadesLogo /> for the icon alone.
 * Use <HadesLogoHorizontal /> for the sidebar horizontal lockup.
 * Use <HadesLogoFavicon /> for 32 px favicon rendering.
 */
export const HadesLogo: React.FC<HadesLogoProps> = ({ size = 40, theme = 'dark', id = 'a' }) => {
  const gId = `hades-g-${id}`;
  const gsId = `hades-gs-${id}`;

  const isDark = theme === 'dark';
  const hexFill = isDark ? '#111827' : '#f0f7ff';
  const c0 = isDark ? '#38bdf8' : '#0ea5e9';
  const c1 = isDark ? '#3b82f6' : '#2563eb';
  const c2 = isDark ? '#8b5cf6' : '#7c3aed';
  const dot1 = isDark ? '#60a5fa' : '#0ea5e9';
  const dot2 = isDark ? '#a78bfa' : '#7c3aed';

  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120" width={size} height={size}>
      <defs>
        <linearGradient id={gId} x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%"   stopColor={c0}/>
          <stop offset="50%"  stopColor={c1}/>
          <stop offset="100%" stopColor={c2}/>
        </linearGradient>
        {isDark && (
          <linearGradient id={gsId} x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%"   stopColor="#0ea5e9" stopOpacity="0.15"/>
            <stop offset="100%" stopColor="#8b5cf6" stopOpacity="0.05"/>
          </linearGradient>
        )}
      </defs>

      {/* Hexagon background */}
      <polygon
        points="60,5 109,32.5 109,87.5 60,115 11,87.5 11,32.5"
        fill={hexFill}
        stroke={`url(#${gId})`}
        strokeWidth="2"
      />
      {isDark && (
        <polygon
          points="60,5 109,32.5 109,87.5 60,115 11,87.5 11,32.5"
          fill={`url(#${gsId})`}
        />
      )}

      {/* Shield outline */}
      <path
        d="M60 22 L88 35 L88 66 C88 83 76 93 60 99 C44 93 32 83 32 66 L32 35 Z"
        fill="none"
        stroke={`url(#${gId})`}
        strokeWidth="2.2"
        strokeLinejoin="round"
      />

      {/* H - left bar */}
      <rect x="50" y="47" width="7" height="26" rx="2" fill={`url(#${gId})`}/>
      {/* H - right bar */}
      <rect x="63" y="47" width="7" height="26" rx="2" fill={`url(#${gId})`}/>
      {/* H - crossbar */}
      <rect x="50" y="57" width="20" height="7"  rx="2" fill={`url(#${gId})`}/>

      {/* Corner dots */}
      <circle cx="53.5" cy="47" r="3" fill={dot1}/>
      <circle cx="66.5" cy="47" r="3" fill={dot2}/>
      <circle cx="53.5" cy="73" r="3" fill={dot2}/>
      <circle cx="66.5" cy="73" r="3" fill={dot1}/>
    </svg>
  );
};

/** 32 px favicon variant - simplified (no dots, thicker stroke) */
export const HadesLogoFavicon: React.FC = () => (
  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
    <defs>
      <linearGradient id="hades-fav-g" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%"   stopColor="#38bdf8"/>
        <stop offset="100%" stopColor="#8b5cf6"/>
      </linearGradient>
    </defs>
    <polygon
      points="16,1 30,8.5 30,23.5 16,31 2,23.5 2,8.5"
      fill="#111827"
      stroke="url(#hades-fav-g)"
      strokeWidth="1.5"
    />
    <rect x="10.5" y="11" width="3.5" height="10" rx="1" fill="url(#hades-fav-g)"/>
    <rect x="18"   y="11" width="3.5" height="10" rx="1" fill="url(#hades-fav-g)"/>
    <rect x="10.5" y="15" width="11"  height="3"  rx="1" fill="url(#hades-fav-g)"/>
  </svg>
);

/**
 * Horizontal lockup: icon + "Hades" / "Schema Registry" side by side.
 * Used in the expanded sidebar.
 */
export const HadesLogoHorizontal: React.FC<{ theme?: 'dark' | 'light'; iconSize?: number }> = ({
  theme = 'dark',
  iconSize = 32,
}) => {
  const nameFg = theme === 'dark' ? '#e6edf3' : '#1f2328';
  const subFg  = theme === 'dark' ? '#8b949e' : '#636c76';

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <HadesLogo size={iconSize} theme={theme} id="horiz"/>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        <span style={{ fontSize: 15, fontWeight: 700, color: nameFg, letterSpacing: -0.5, lineHeight: 1 }}>
          Hades
        </span>
        <span style={{ fontSize: 10, fontWeight: 500, color: subFg, letterSpacing: 3, textTransform: 'uppercase', lineHeight: 1 }}>
          Schema Registry
        </span>
      </div>
    </div>
  );
};

export default HadesLogo;
