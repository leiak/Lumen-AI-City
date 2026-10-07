'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import clsx from 'clsx';

const navItems = [
  { href: '/', label: 'Dashboard' },
  { href: '/wallet', label: 'Wallet' },
  { href: '/transactions', label: 'Transactions' },
  { href: '/admin', label: 'Admin Tools' },
  { href: '/bt-editor', label: 'BT Editor' },
  { href: '/saga-viz', label: 'Saga Viz' },
];

export function Sidebar() {
  const pathname = usePathname();
  return (
    <aside className="w-56 bg-white border-r border-gray-200 p-4">
      <nav className="space-y-1">
        {navItems.map((item) => (
          <Link
            key={item.href}
            href={item.href as any}
            data-testid={`nav-${item.label.toLowerCase().replace(' ', '-')}`}
            className={clsx(
              'block px-3 py-2 rounded text-sm transition',
              pathname === item.href
                ? 'bg-blue-50 text-blue-700 font-medium'
                : 'text-gray-700 hover:bg-gray-100'
            )}
          >
            {item.label}
          </Link>
        ))}
      </nav>
    </aside>
  );
}