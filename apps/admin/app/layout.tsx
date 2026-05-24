import type { Metadata } from 'next';
import localFont from 'next/font/local';
import './globals.css';
import Link from 'next/link';

const geistSans = localFont({
  src: './fonts/GeistVF.woff',
  variable: '--font-geist-sans',
});
const geistMono = localFont({
  src: './fonts/GeistMonoVF.woff',
  variable: '--font-geist-mono',
});

export const metadata: Metadata = {
  title: 'Camaron Admin',
  description: 'Camera streaming admin dashboard',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable}`}>
        <nav className="nav">
          <div className="nav-inner">
            <Link href="/" className="nav-brand">
              Camaron
            </Link>
            <div className="nav-links">
              <Link href="/">Dashboard</Link>
              <Link href="/cameras">Cameras</Link>
            </div>
          </div>
        </nav>
        <main className="main">{children}</main>
      </body>
    </html>
  );
}
