import React from 'react';

const Divider: React.FC<{ style?: React.CSSProperties }> = ({ style }) => (
  <div style={{ borderTop: '1px solid var(--c-border)', ...style }}/>
);

export default Divider;
