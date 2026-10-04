export function NavIcon({ name }: { name: 'home' | 'raids' | 'players' | 'polls' | 'settings' | 'series' | 'import' | 'profile' }) {
  const paths = {
    home: 'm3 10 9-7 9 7M5 9v12h5v-7h4v7h5V9',
    raids: 'M5 5h14v16H5zM8 3v4m8-4v4M5 10h14M8 14h2m4 0h2m-8 3h2',
    players: 'M16 21v-3a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v3m18 0v-3a4 4 0 0 0-3-4M9 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8m8 0a4 4 0 0 1 0 8',
    polls: 'M4 21V11h4v10m4 0V3h4v18m4 0v-7h2v7M2 21h21',
    settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M9 3h6l1 4 4 1v8l-4 1-1 4H9l-1-4-4-1V8l4-1Z',
    series: 'M4 9a8 8 0 0 1 14-4l3 3m0-5v5h-5M20 15a8 8 0 0 1-14 4l-3-3m0 5v-5h5',
    import: 'M12 3v13m-5-5 5 5 5-5M4 17v4h16v-4',
    profile: 'M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8M4 21v-3a5 5 0 0 1 5-5h6a5 5 0 0 1 5 5v3Z',
  }
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[name]} /></svg>
}
