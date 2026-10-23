import type { ReactNode } from 'react'
import { useOwnMFAState } from '../accountSecurity'
import { useAuth } from '../auth'
import { NavIcon } from '../components/NavIcon'
import { Link } from '../router'

// Why a second factor matters (ADR 092). Every number on this page is taken from
// the published source listed under "Quellen"; ADR 026 forbids invented figures.

interface Source {
  id: string
  title: string
  publisher: string
  url: string
}

const accountSecuritySources: Source[] = [
  {
    id: 'microsoft-2019',
    title: 'One simple action you can take to prevent 99.9 percent of attacks on your accounts (20.08.2019)',
    publisher: 'Microsoft Security Blog',
    url: 'https://www.microsoft.com/en-us/security/blog/2019/08/20/one-simple-action-you-can-take-to-prevent-99-9-percent-of-account-attacks/',
  },
  {
    id: 'microsoft-2023',
    title: 'How effective is multifactor authentication at deterring cyberattacks? (Meyer u. a., 2023)',
    publisher: 'Microsoft Research',
    url: 'https://www.microsoft.com/en-us/research/publication/how-effective-is-multifactor-authentication-at-deterring-cyberattacks/',
  },
  {
    id: 'google-2019',
    title: 'New research: How effective is basic account hygiene at preventing hijacking (17.05.2019, mit NYU und UC San Diego)',
    publisher: 'Google Security Blog',
    url: 'https://security.googleblog.com/2019/05/new-research-how-effective-is-basic.html',
  },
  {
    id: 'verizon-2025',
    title: '2025 Data Breach Investigations Report',
    publisher: 'Verizon',
    url: 'https://www.verizon.com/about/news/2025-data-breach-investigations-report',
  },
  {
    id: 'bsi-2fa',
    title: 'Zwei-Faktor-Authentisierung',
    publisher: 'Bundesamt für Sicherheit in der Informationstechnik (BSI)',
    url: 'https://www.bsi.bund.de/DE/Themen/Verbraucherinnen-und-Verbraucher/Informationen-und-Empfehlungen/Cyber-Sicherheitsempfehlungen/Accountschutz/Zwei-Faktor-Authentisierung/zwei-faktor-authentisierung_node.html',
  },
  {
    id: 'bsi-bewertung',
    title: 'Wie sicher und einfach sind Verfahren der Zwei-Faktor-Authentisierung?',
    publisher: 'Bundesamt für Sicherheit in der Informationstechnik (BSI)',
    url: 'https://www.bsi.bund.de/DE/Themen/Verbraucherinnen-und-Verbraucher/Informationen-und-Empfehlungen/Cyber-Sicherheitsempfehlungen/Accountschutz/Zwei-Faktor-Authentisierung/Bewertung-2FA-Verfahren/bewertung-2fa-verfahren_node.html',
  },
  {
    id: 'cisa-mfa',
    title: 'More than a Password',
    publisher: 'US Cybersecurity and Infrastructure Security Agency (CISA)',
    url: 'https://www.cisa.gov/MFA',
  },
  {
    id: 'hibp',
    title: 'Have I Been Pwned: prüfen, ob deine E-Mail-Adresse in einem Datenleck steckt',
    publisher: 'Troy Hunt',
    url: 'https://haveibeenpwned.com/',
  },
]

function Cite({ id }: { id: string }) {
  const index = accountSecuritySources.findIndex((source) => source.id === id)
  return <sup className="security-cite"><a href={`#quelle-${id}`} onClick={(event) => {
    event.preventDefault()
    document.getElementById(`quelle-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }} aria-label={`Quelle ${index + 1}`}>[{index + 1}]</a></sup>
}

function Stat({ value, children, source }: { value: string, children: ReactNode, source: string }) {
  return <article className="security-stat">
    <strong>{value}</strong>
    <p>{children} <Cite id={source} /></p>
  </article>
}

function OwnStatus() {
  const { player } = useAuth()
  const mfaState = useOwnMFAState(player !== null)
  if (!player) {
    return <section className="card security-status">
      <p>Melde dich an, um zu sehen, ob dein Konto einen zweiten Faktor hat.</p>
      <Link className="button-link primary" to="/login">Anmelden</Link>
    </section>
  }
  if (mfaState === 'loading') return <section className="card security-status"><p className="muted">Prüfe dein Konto…</p></section>
  if (mfaState === 'protected') {
    return <section className="card security-status ok">
      <NavIcon name="check" />
      <p><strong>Dein Konto hat einen zweiten Faktor.</strong> Danke! Bewahre deine Wiederherstellungscodes gut auf.</p>
      <Link className="button-link" to={`/players/${player.id}/konto`}>Konto ansehen</Link>
    </section>
  }
  return <section className="card security-status warn">
    <NavIcon name="alertTriangle" />
    <p><strong>Dein Konto hat noch keinen zweiten Faktor.</strong> Das Einrichten dauert etwa zwei Minuten.</p>
    <Link className="button-link primary" to={`/players/${player.id}/konto`}>Jetzt einrichten</Link>
  </section>
}

export function AccountSecurityPage() {
  return <div className="guide-page security-page">
    <div className="page-heading">
      <div>
        <p className="eyebrow">Kontosicherheit</p>
        <h1>Warum ein zweiter Faktor?</h1>
      </div>
    </div>
    <p className="guide-lead">Erinnerst du dich an den Blizzard Authenticator, für den es sogar ein Haustier gab? Genau das ist ein zweiter Faktor (auch MFA oder 2FA genannt): Neben dem Passwort brauchst du einen Code von deinem Handy oder einen Sicherheitsschlüssel. Blizzard hat ihn nicht ohne Grund eingeführt. Wer früher nur ein Passwort hatte, stand nach einem Hack oft ohne Gold und Ausrüstung in Dalaran. Hier gilt dasselbe: Wer nur dein Passwort kennt, kommt ohne den zweiten Faktor nicht hinein.</p>

    <OwnStatus />

    <section className="card guide-section" aria-labelledby="zahlen-title">
      <h2 id="zahlen-title">Was die Zahlen sagen</h2>
      <p>Große Anbieter werten jeden Tag Millionen Anmeldeversuche aus. Ihre Ergebnisse sind eindeutig:</p>
      <div className="security-stats">
        <Stat value="99,9 %" source="microsoft-2019">der Angriffe auf Konten lassen sich laut Microsoft mit einem zweiten Faktor blockieren.</Stat>
        <Stat value="99,22 %" source="microsoft-2023">weniger Risiko einer Kontoübernahme fand eine Microsoft-Studie mit echten Konten. Selbst wenn das Passwort schon geleakt war, sank das Risiko um 98,56 %.</Stat>
        <Stat value="100 %" source="google-2019">der automatisierten Bot-Angriffe auf Konten stoppte bei Google schon ein einfacher Code als zweiter Faktor.</Stat>
        <Stat value="96 %" source="google-2019">der Phishing-Massenmails liefen bei Google ins Leere, sobald ein Code als zweiter Faktor nötig war.</Stat>
        <Stat value="22 %" source="verizon-2025">der untersuchten Datenlecks begannen laut Verizon-Report 2025 mit gestohlenen Zugangsdaten. Das war der häufigste Weg hinein.</Stat>
        <Stat value="73 %" source="microsoft-2019">der Passwörter sind laut Microsoft Duplikate, also an mehreren Stellen gleich.</Stat>
      </div>
      <p className="guide-tip">Microsoft zählt nach eigenen Angaben über 300 Millionen betrügerische Anmeldeversuche pro Tag allein auf seinen Cloud-Diensten <Cite id="microsoft-2019" />. Angreifer probieren nicht gezielt dich aus, sondern Passwortlisten gegen alles, was sie finden.</p>
    </section>

    <section className="card guide-section" aria-labelledby="warum-title">
      <h2 id="warum-title">Warum gerade hier?</h2>
      <p>Die meisten Konten werden nicht „gehackt“, sondern mit einem Passwort geöffnet, das woanders verloren ging. Wenn du dasselbe Passwort auch für ein Forum, Discord oder ein anderes Spiel benutzt und dort ein Datenleck passiert, probieren Bots es automatisch auch auf anderen Seiten aus. Das nennt man <em>Credential Stuffing</em>.</p>
      <p>Mit deinem Konto könnte jemand hier zum Beispiel:</p>
      <ul>
        <li>dich in deinem Namen für Raids an- oder abmelden,</li>
        <li>deine Wunschliste ändern oder deine Item-Locks aufheben,</li>
        <li>in Umfragen für dich abstimmen,</li>
        <li>bei Raidleitung und Admins zusätzlich Loot, Aufstellungen und Konten anderer verändern.</li>
      </ul>
      <p>Jede Änderung steht zwar im Protokoll, aber aufräumen kostet die Gilde Zeit und Nerven. Mit einem zweiten Faktor kommt es gar nicht so weit.</p>
    </section>

    <section className="card guide-section" aria-labelledby="methoden-title">
      <h2 id="methoden-title">Welche Methode passt zu dir?</h2>
      <p>Hier gibt es zwei Möglichkeiten. Du kannst auch beide einrichten.</p>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Methode</th><th>So funktioniert es</th><th>Schutz</th></tr></thead>
          <tbody>
            <tr><td><strong>Sicherheitsschlüssel</strong><br /><span className="muted">YubiKey, FIDO2, Passkey-fähiges Gerät</span></td><td>Beim Anmelden steckst du den Schlüssel ein oder tippst ihn an.</td><td><span className="badge ok">Am stärksten</span> Schützt auch vor gefälschten Login-Seiten, weil der Schlüssel nur mit der echten Seite spricht <Cite id="bsi-bewertung" />. In Googles Untersuchung fiel niemand, der nur Sicherheitsschlüssel nutzte, auf gezieltes Phishing herein <Cite id="google-2019" />.</td></tr>
            <tr><td><strong>Authenticator-App</strong><br /><span className="muted">z. B. Google Authenticator, Microsoft Authenticator, Aegis, 2FAS</span></td><td>Die App zeigt alle 30 Sekunden einen neuen 6-stelligen Code.</td><td><span className="badge">Sehr gut</span> Kostenlos und für fast jeden machbar. Gib den Code nur auf dieser Seite ein, nie in einem Chat oder auf einer anderen Seite <Cite id="bsi-bewertung" />.</td></tr>
          </tbody>
        </table>
      </div>
      <p>Das BSI empfiehlt, einen zweiten Faktor überall zu nutzen, wo er angeboten wird <Cite id="bsi-2fa" />. Die US-Behörde CISA sieht das genauso <Cite id="cisa-mfa" />.</p>
    </section>

    <section className="card guide-section" aria-labelledby="einrichten-title">
      <h2 id="einrichten-title">In zwei Minuten eingerichtet</h2>
      <ol>
        <li>Oben rechts auf deinen Namen klicken, dann den Reiter <strong>Konto</strong> öffnen.</li>
        <li>Unter <strong>Multi-Faktor-Authentifizierung</strong> auf <strong>Authenticator einrichten</strong> klicken und dein aktuelles Passwort bestätigen.</li>
        <li>Den QR-Code mit deiner Authenticator-App scannen.</li>
        <li>Den 6-stelligen Code aus der App eintragen und <strong>MFA aktivieren</strong> klicken.</li>
        <li>Die <strong>Wiederherstellungscodes</strong> kopieren und sicher aufbewahren, z. B. im Passwortmanager. Sie werden nur dieses eine Mal angezeigt und helfen, wenn dein Handy weg ist.</li>
      </ol>
      <p>Einen Sicherheitsschlüssel registrierst du im selben Bereich mit <strong>Sicherheitsschlüssel registrieren</strong>.</p>
    </section>

    <section className="card guide-section" aria-labelledby="tipps-title">
      <h2 id="tipps-title">Noch drei einfache Tipps</h2>
      <ul>
        <li><strong>Ein Passwort pro Seite.</strong> Ein Passwortmanager merkt sie sich für dich. Hier gilt: mindestens 12 Zeichen.</li>
        <li><strong>Prüfe, ob du betroffen bist.</strong> Auf <a href="https://haveibeenpwned.com/" target="_blank" rel="noreferrer">Have I Been Pwned</a> siehst du, ob deine E-Mail-Adresse in einem bekannten Datenleck steckt <Cite id="hibp" />.</li>
        <li><strong>Niemand aus der Gilde fragt nach deinem Passwort oder Code.</strong> Auch kein Admin. Wer danach fragt, will dein Konto.</li>
      </ul>
    </section>

    <section className="card guide-section security-sources" aria-labelledby="quellen-title">
      <h2 id="quellen-title">Quellen</h2>
      <ol>
        {accountSecuritySources.map((source) => <li key={source.id} id={`quelle-${source.id}`}>
          <a href={source.url} target="_blank" rel="noreferrer">{source.title}</a> <span className="muted">· {source.publisher}</span>
        </li>)}
      </ol>
    </section>
  </div>
}
