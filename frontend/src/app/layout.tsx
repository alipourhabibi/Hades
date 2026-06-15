import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Hades Schema Registry',
  description: 'Open-source Buf-compatible Protobuf schema registry',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
              try {
                var t = document.cookie.match(/hades_theme=([^;]+)/);
                if (t && t[1] === 'light') document.documentElement.classList.add('light');
              } catch(e) {}
            `,
          }}
        />
      </head>
      <body suppressHydrationWarning>{children}</body>
    </html>
  );
}
