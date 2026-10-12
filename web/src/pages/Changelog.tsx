import { changelog } from '../release'

export function ChangelogPage() {
  return <div className="changelog-page">
    <div className="page-heading">
      <div>
        <p className="eyebrow">Versionsverlauf</p>
        <h1>Changelog</h1>
      </div>
    </div>
    {changelog.map((release) => <section className="card changelog-release" key={release.version}>
      <div className="section-heading">
        <h2>Version {release.version}</h2>
        <span className="badge ok">{release.title}</span>
      </div>
      <ul>
        {release.changes.map((change) => <li key={change}>{change}</li>)}
      </ul>
    </section>)}
  </div>
}
