export function NavIcon({ name }: { name: 'home' | 'raids' | 'players' | 'polls' | 'settings' | 'series' | 'import' | 'profile' | 'tank' | 'heal' | 'dd' | 'bell' | 'chart' | 'calendar' | 'loot' | 'shield' }) {
  const paths: Record<string, string> = {
    home: 'm3 10 9-7 9 7M5 9v12h5v-7h4v7h5V9',
    raids: 'M5 5h14v16H5zM8 3v4m8-4v4M5 10h14M8 14h2m4 0h2m-8 3h2',
    players: 'M16 21v-3a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v3m18 0v-3a4 4 0 0 0-3-4M9 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8m8 0a4 4 0 0 1 0 8',
    polls: 'M4 21V11h4v10m4 0V3h4v18m4 0v-7h2v7M2 21h21',
    settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M9 3h6l1 4 4 1v8l-4 1-1 4H9l-1-4-4-1V8l4-1Z',
    series: 'M4 9a8 8 0 0 1 14-4l3 3m0-5v5h-5M20 15a8 8 0 0 1-14 4l-3-3m0 5v-5h5',
    import: 'M12 3v13m-5-5 5 5 5-5M4 17v4h16v-4',
    profile: 'M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8M4 21v-3a5 5 0 0 1 5-5h6a5 5 0 0 1 5 5v3Z',
    tank: 'M12 2l8 4v6c0 5-3.5 8.5-8 10-4.5-1.5-8-5-8-10V6z',
    heal: 'M12 2v20M2 12h20',
    dd: 'M12 2l3 6 7 1-5 5 1 7-6-3-6 3 1-7-5-5 7-1z',
    bell: 'M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M10 21a2 2 0 0 0 4 0',
    chart: 'M3 3v18h18M8 17V9m5 8V5m5 12v-6',
    calendar: 'M8 2v4m8-4v4M3 8h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z',
    loot: 'M21 8l-9-5-9 5v8l9 5 9-5zM3 8l9 5 9-5M12 13v8',
    shield: 'M12 2l8 4v6c0 5-3.5 8.5-8 10-4.5-1.5-8-5-8-10V6z',
  }
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[name]} /></svg>
}
