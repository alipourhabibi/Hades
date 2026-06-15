'use client';
import React, { useState } from 'react';

interface CardProps {
  children: React.ReactNode;
  style?: React.CSSProperties;
  onClick?: () => void;
  hover?: boolean;
}

const Card: React.FC<CardProps> = ({ children, style, onClick, hover }) => {
  const [hov, setHov] = useState(false);
  return (
    <div onClick={onClick}
      onMouseEnter={() => setHov(true)} onMouseLeave={() => setHov(false)}
      style={{
        background: 'var(--c-bg-default)',
        border: `1px solid ${hov && hover ? 'var(--c-accent)' : 'var(--c-border)'}`,
        borderRadius: 8, transition: 'border-color 0.15s',
        cursor: onClick ? 'pointer' : 'default', ...style,
      }}>
      {children}
    </div>
  );
};

export default Card;
