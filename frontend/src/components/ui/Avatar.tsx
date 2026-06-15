import React from 'react';

interface AvatarProps {
  initials?: string;
  size?: number;
  color?: string;
  style?: React.CSSProperties;
}

const COLORS = ['#1f6feb','#388bfd','#8957e5','#2ea043','#da3633','#9e6a03','#6e7681'];

const Avatar: React.FC<AvatarProps> = ({ initials = '?', size = 28, color, style }) => {
  const hash = initials.charCodeAt(0) + (initials.charCodeAt(1) || 0);
  const bg = color || COLORS[hash % COLORS.length];
  return (
    <div style={{
      width: size, height: size, borderRadius: size >= 32 ? 8 : 4,
      background: bg, color: '#fff',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontSize: size * 0.36, fontWeight: 600, letterSpacing: -0.5,
      flexShrink: 0, userSelect: 'none', ...style,
    }}>
      {initials.slice(0, 2).toUpperCase()}
    </div>
  );
};

export default Avatar;
