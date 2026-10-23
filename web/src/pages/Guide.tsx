import type { ReactNode } from 'react'
import { useAuth } from '../auth'
import { Link } from '../router'
import type { Role } from '../types'

// The user guide. Screenshots live in public/anleitung and are regenerated with
// DOCS_TOUR=1 npx playwright test e2e/zz-docs-tour.spec.ts (ADR 083).

type Audience = 'alle' | 'mitglieder' | 'raidleitung' | 'admins'

const audiences: { id: Audience, label: string, hint: string, first: string }[] = [
  { id: 'alle', label: 'Neu hier', hint: 'Konto bekommen, einloggen, Menü', first: 'erste-schritte' },
  { id: 'mitglieder', label: 'Mitglieder', hint: 'Anmelden, absagen, Wunschliste, EP', first: 'anmelden' },
  { id: 'raidleitung', label: 'Raidleitung', hint: 'Raids planen, Aufstellung, Loot', first: 'raids-anlegen' },
  { id: 'admins', label: 'Admins', hint: 'Spieler, Registrierung, Protokoll', first: 'admins' },
]

const roleAudience: Record<Role, Audience> = {
  GUEST: 'alle',
  MEMBER: 'mitglieder',
  RAIDASSISTENT: 'raidleitung',
  RAIDLEADER: 'raidleitung',
  ADMIN: 'admins',
}

function jumpTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function Shot({ name, caption }: { name: string, caption: string }) {
  const src = `/anleitung/${name}.webp`
  return <figure className="guide-shot">
    <a href={src} target="_blank" rel="noreferrer" title="Bild in voller Größe öffnen">
      <img src={src} alt={caption} loading="lazy" width={1280} height={960} />
    </a>
    <figcaption>{caption}</figcaption>
  </figure>
}

function Tip({ children }: { children: ReactNode }) {
  return <p className="guide-tip">{children}</p>
}

function Section({ id, audience, title, children }: { id: string, audience: Audience, title: string, children: ReactNode }) {
  const label = audiences.find((entry) => entry.id === audience)?.label
  return <section className="card guide-section" id={id} aria-labelledby={`${id}-title`}>
    <div className="section-heading">
      <h2 id={`${id}-title`}>{title}</h2>
      <span className="badge">{label}</span>
    </div>
    {children}
    <button type="button" className="guide-top" onClick={() => jumpTo('anleitung-start')}>Nach oben</button>
  </section>
}

const contents: { id: string, title: string, audience: Audience }[] = [
  { id: 'rollen', title: 'Wer darf was?', audience: 'alle' },
  { id: 'erste-schritte', title: 'Konto bekommen und einloggen', audience: 'alle' },
  { id: 'menue', title: 'Das Menü links', audience: 'alle' },
  { id: 'anmelden', title: 'Anmelden, absagen, unsicher', audience: 'mitglieder' },
  { id: 'wunschliste', title: 'Wunschliste und Addon-Export', audience: 'mitglieder' },
  { id: 'ep', title: 'EP, Item-Locks, Konto und Umfragen', audience: 'mitglieder' },
  { id: 'schattengram', title: 'Schattengram: Questreihe und Reihenfolge', audience: 'alle' },
  { id: 'raids-anlegen', title: 'Raids anlegen und vorausplanen', audience: 'raidleitung' },
  { id: 'aufstellung', title: 'Anmeldungen, Aufstellung und Gesuchtext', audience: 'raidleitung' },
  { id: 'loot', title: 'Loot eintragen und archivieren', audience: 'raidleitung' },
  { id: 'admins', title: 'Spieler, Registrierung, Raidserien, Protokoll', audience: 'admins' },
  { id: 'fragen', title: 'Häufige Fragen', audience: 'alle' },
]

export function GuidePage() {
  const { player } = useAuth()
  const mine = player ? roleAudience[player.role] : 'alle'
  return <div className="guide-page" id="anleitung-start">
    <div className="page-heading">
      <div>
        <p className="eyebrow">Hilfe</p>
        <h1>Anleitung</h1>
      </div>
    </div>
    <p className="guide-lead">Hier steht Schritt für Schritt, wie die Raidverwaltung funktioniert, mit Bildern und Beispielen. Du musst nicht alles lesen: Wähle einfach deinen Bereich.</p>

    <section className="card guide-jump" aria-label="Bereich wählen">
      <h2>Wofür brauchst du Hilfe?</h2>
      <div className="guide-jump-grid">
        {audiences.map((entry) => <button type="button" key={entry.id} className={entry.id === mine ? 'guide-jump-card mine' : 'guide-jump-card'} onClick={() => jumpTo(entry.first)}>
          <strong>{entry.label}</strong>
          <span>{entry.hint}</span>
          {entry.id === mine && player && <em>Dein Bereich</em>}
        </button>)}
      </div>
      <details className="guide-contents">
        <summary>Alle Kapitel anzeigen</summary>
        <ol>
          {contents.map((entry) => <li key={entry.id}><button type="button" className="link-button" onClick={() => jumpTo(entry.id)}>{entry.title}</button></li>)}
        </ol>
      </details>
    </section>

    <Section id="rollen" audience="alle" title="Wer darf was?">
      <p>Jeder Account hat eine <strong>Rolle</strong>. Du siehst sie oben rechts neben deinem Namen{player ? <> (bei dir: <code>{player.role}</code>)</> : null}. Höhere Rollen können immer alles, was die Rollen darunter können.</p>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Rolle</th><th>Wer ist das?</th><th>Was kann ich?</th></tr></thead>
          <tbody>
            <tr><td><strong>Besucher</strong></td><td>Nicht eingeloggt</td><td>Nächsten Raid, Raidhistorie, aktive Umfrage und diese Anleitung ansehen</td></tr>
            <tr><td><strong>Gast</strong> <code>GUEST</code></td><td>Frisch registriert</td><td>Alles ansehen, auch die Schattengram-Reihenfolge. Anmelden geht erst, wenn ein Admin dich zum Mitglied macht.</td></tr>
            <tr><td><strong>Mitglied</strong> <code>MEMBER</code></td><td>Normale Raider</td><td>Für Raids anmelden, absagen oder „unsicher“ melden, Wunschliste, Item-Locks, abstimmen</td></tr>
            <tr><td><strong>Raidassistent</strong> <code>RAIDASSISTENT</code></td><td>Helfer der Raidleitung</td><td>Dazu: Raids anlegen und vorausplanen, Anmeldungen übernehmen, Aufstellung, Gesuchtext, Loot, archivieren, Umfragen erstellen, Schattengram-Liste pflegen und pro Raid einteilen</td></tr>
            <tr><td><strong>Raidleitung</strong> <code>RAIDLEADER</code></td><td>Leitet die Raids</td><td>Alles vom Raidassistenten</td></tr>
            <tr><td><strong>Admin</strong> <code>ADMIN</code></td><td>Gildenverwaltung</td><td>Dazu: Spieler und Rollen, Registrierungscodes, Raidserien, Gildenkonfiguration, Audit-Protokoll</td></tr>
          </tbody>
        </table>
      </div>
      <Tip><strong>Beispiel-Gilde in den Bildern:</strong> Thorgrim ist Raidleiter (Krieger-Tank), Mira ist Raidassistentin (Paladin-Heilerin), Sternfall ist Mitglied (Schattenpriester mit dem Twink Morgentau). Die Raidserie heißt „ICC25 Donnerstag“.</Tip>
    </Section>

    <Section id="erste-schritte" audience="alle" title="Konto bekommen und einloggen">
      <p>Ein Konto bekommst du über einen <strong>Code</strong>, den dir ein Admin ingame schickt. So ist sicher, dass der Charakter wirklich dir gehört.</p>
      <h3>Schritt 1: Code anfordern</h3>
      <ol>
        <li>Klicke auf der Login-Seite auf <strong>Code anfordern</strong>.</li>
        <li>Gib mindestens 3 Buchstaben deines Rising-Gods-Charakters ein und klicke auf <strong>Suchen</strong>.</li>
        <li>Wähle deinen Charakter und klicke auf <strong>Anfrage erstellen</strong>.</li>
        <li>Warte, bis dir ein Admin den Code <strong>ingame</strong> an genau diesen Charakter schickt.</li>
      </ol>
      <Shot name="02-code-anfordern" caption="Code anfordern: Charakter suchen und Anfrage erstellen" />
      <h3>Schritt 2: Registrieren</h3>
      <ol>
        <li>Klicke auf <strong>Registrieren</strong>.</li>
        <li>Trage den <strong>Code</strong> aus dem Spiel ein.</li>
        <li>Wähle einen <strong>Benutzernamen</strong> und ein <strong>Passwort</strong>.</li>
        <li>Klicke auf <strong>Registrieren</strong>.</li>
      </ol>
      <Shot name="03-registrieren" caption="Registrieren mit Code, Benutzername und Passwort" />
      <Tip><strong>Wichtig:</strong> Nach der Registrierung bist du zuerst <strong>Gast</strong>. Sag einem Admin Bescheid, damit er dich zum <strong>Mitglied</strong> macht. Erst dann kannst du dich für Raids anmelden und eine Wunschliste anlegen.</Tip>
      <h3>Schritt 3: Einloggen</h3>
      <p>Ab jetzt gehst du immer über <strong>Anmelden</strong>: Benutzername und Passwort eingeben, fertig.</p>
      <Shot name="01-login" caption="Die Login-Seite" />
      <p>Auch ohne Login siehst du die Übersicht mit dem nächsten Raid, der Aufstellung, der aktiven Umfrage und der Raidhistorie.</p>
      <Shot name="04-oeffentliche-uebersicht" caption="Übersicht ohne Login" />
    </Section>

    <Section id="menue" audience="alle" title="Das Menü links">
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Menüpunkt</th><th>Wofür?</th></tr></thead>
          <tbody>
            <tr><td><strong>Übersicht</strong></td><td>Deine nächsten Raids mit Anmelde-Knöpfen, der nächste Raid, Umfrage, Neuigkeiten</td></tr>
            <tr><td><strong>Raids</strong></td><td>Alle geplanten und vergangenen Raids. Hier öffnest du einen Raid im Detail.</td></tr>
            <tr><td><strong>Aktueller Raid</strong></td><td>Direkt zum Raid von heute Abend: der neueste Raid, der schon begonnen hat (sonst der nächste). Auf dem Handy heißt er <strong>Aktuell</strong>.</td></tr>
            <tr><td><strong>Spieler</strong></td><td>Alle Spieler. Über deinen Namen (auch oben rechts) kommst du zu deinem Profil.</td></tr>
            <tr><td><strong>Statistiken</strong></td><td>Auswertungen über Raids und Loot</td></tr>
            <tr><td><strong>Umfragen</strong></td><td>Abstimmen, z. B. über einen neuen Raidtag</td></tr>
            <tr><td><strong>Beute</strong></td><td>Item-Katalog. Hier fügst du Items zur Wunschliste hinzu.</td></tr>
            <tr><td><strong>Schattengram</strong></td><td>Wer an Schattengram arbeitet, bei welchem Schritt, in welcher Reihenfolge, und wer schon fertig ist.</td></tr>
            <tr><td><strong>Meine Wunschliste</strong></td><td>Wunschliste ordnen und fürs Addon exportieren</td></tr>
            <tr><td><strong>Addons</strong></td><td>RaidLedger und RaidWishlist herunterladen, Installation und Anleitung</td></tr>
            <tr><td><strong>Anleitung</strong></td><td>Diese Seite</td></tr>
          </tbody>
        </table>
      </div>
      <p>Unten links steht die Version. Ein Klick darauf zeigt, was zuletzt neu dazugekommen ist.</p>
      <Shot name="11-uebersicht-unten" caption="Übersicht: nächster Raid, Aufstellung, Anwesenheit, Umfrage, Raidhistorie" />
    </Section>

    <Section id="anmelden" audience="mitglieder" title="Anmelden, absagen, unsicher">
      <p>Das Wichtigste zuerst: <strong>Alles geht direkt auf der <Link to="/">Übersicht</Link></strong> im Kasten <em>Meine nächsten Raids</em>. Jeder kommende Raid hat eine eigene Karte: oben Name und Termin, darunter dein <strong>Status</strong>, darunter die Knöpfe.</p>
      <Shot name="10-uebersicht-mitglied" caption="Meine nächsten Raids: eine Karte pro Raid" />
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Status</th><th>Bedeutung</th></tr></thead>
          <tbody>
            <tr><td><span className="badge warn">Noch nicht angemeldet</span></td><td>Du hast noch nichts gesagt.</td></tr>
            <tr><td><span className="badge">Angemeldet</span></td><td>Die Raidleitung weiß, dass du willst, hat dich aber noch nicht eingeplant.</td></tr>
            <tr><td><span className="badge ok">Bestätigt</span></td><td>Du bist in der Aufstellung.</td></tr>
            <tr><td><span className="badge warn">Unsicher</span></td><td>Du weißt noch nicht sicher, ob du kommst.</td></tr>
            <tr><td><span className="badge bad">Abgesagt</span></td><td>Du kannst nicht.</td></tr>
          </tbody>
        </table>
      </div>
      <h3>Anmelden mit einem Klick</h3>
      <p>Klicke auf <strong>Mit ‹Charakter› (‹Spec›) anmelden</strong>. Die Website nimmt deinen Main mit seinem Spec.</p>
      <p><strong>Beispiel:</strong> Sternfall klickt bei ICC25-2 auf <em>Mit Sternfall (Schatten) anmelden</em>. Danach zeigt die Karte <em>Angemeldet · mit Sternfall (Schatten)</em>.</p>
      <h3>Absagen: „Kann nicht“</h3>
      <ol>
        <li>Klicke auf der Karte auf <strong>Kann nicht</strong>.</li>
        <li>Schreib optional einen Grund dazu, z. B. <em>Familienfeier</em>.</li>
        <li>Klicke auf <strong>Absage senden</strong>.</li>
      </ol>
      <Shot name="12-absage-mit-grund" caption="Absage mit Grund" />
      <h3>Noch nicht sicher: „Unsicher“</h3>
      <p>Klicke auf <strong>Unsicher</strong> und schreib einen kurzen Hinweis, z. B. <em>komme evtl. später</em>. Die Raidleitung sieht das sofort. Sobald du es weißt, klickst du auf <strong>Sicher dabei</strong> oder <strong>Kann nicht</strong>.</p>
      <Shot name="13-angemeldet-und-unsicher" caption="Links angemeldet, in der Mitte als unsicher gemeldet" />
      <Tip><strong>Tipp:</strong> Du kannst deinen Status ändern, solange die Anmeldung offen ist. Eine Absage oder „unsicher“ löscht deine Anmeldung nicht. Die Raidleitung entscheidet, wer mitkommt.</Tip>
      <h3>Mit Twink oder anderem Spec anmelden</h3>
      <ol>
        <li>Klicke auf der Karte auf <strong>Anderer Charakter oder Spec</strong>.</li>
        <li>Setze unter <em>Meine Charaktere anmelden / Spec ändern</em> ein Häkchen bei jedem Charakter, den du anbieten willst.</li>
        <li>Wähle für jeden Charakter den gewünschten <strong>Spec</strong>.</li>
        <li>Klicke auf <strong>Ausgewählte Charaktere anmelden</strong>.</li>
      </ol>
      <p><strong>Beispiel:</strong> Sternfall bietet zusätzlich seinen Schamanen-Twink <em>Morgentau</em> als <em>Wiederherstellung</em> an. Die Raidleitung nimmt dann den, der besser passt.</p>
      <Shot name="14-raid-anmeldungen-mitglied" caption="Raid, Reiter Anmeldungen: dein Status und wer abgesagt hat" />
      <Shot name="15-raid-anmeldungen-formular" caption="Charaktere wählen, Spec wählen, absagen oder unsicher melden" />
    </Section>

    <Section id="wunschliste" audience="mitglieder" title="Wunschliste und Addon-Export">
      <p>Das geht in zwei Schritten: Items im Katalog <strong>hinzufügen</strong>, dann unter <em>Meine Wunschliste</em> <strong>ordnen und exportieren</strong>.</p>
      <h3>Schritt 1: Items hinzufügen</h3>
      <ol>
        <li>Klicke links auf <Link to="/loot-catalog">Beute</Link>.</li>
        <li>Filtere nach Instanz, Boss, Größe und Modus oder suche nach dem Namen, z. B. <em>Bryntroll</em>.</li>
        <li>Unter jedem Item steht <strong>Wunschliste</strong> mit einem Knopf pro Charakter. Klicke auf den Charakter, für den du das Item willst.</li>
        <li>Oben erscheint eine Bestätigung mit dem Knopf <strong>Meine Wunschliste &amp; Addon-Export</strong>.</li>
      </ol>
      <Shot name="16-beute-wunsch" caption="Item hinzufügen: auf den Charakter klicken" />
      <Tip><strong>Achtung:</strong> Heroisch und Normal sind verschiedene Items. Klick die richtige Variante an.</Tip>
      <h3>Schritt 2: Ordnen</h3>
      <ol>
        <li>Klicke links auf <Link to="/wishlist">Meine Wunschliste</Link> und wähle oben den Charakter.</li>
        <li>Stelle pro Item <strong>Main-Spec</strong> oder <strong>Second-Spec</strong> ein.</li>
        <li>Setze <strong>Enhanced</strong>, wenn du dafür EP einsetzen willst.</li>
        <li>Mit <strong>Nach oben</strong> kommt das wichtigste Item nach oben. Die Reihenfolge ist deine Priorität.</li>
      </ol>
      <Shot name="17-wunschliste" caption="Meine Wunschliste mit dem Export-Kasten" />
      <h3>Schritt 3: Ins Addon übernehmen</h3>
      <ol>
        <li>Klicke auf <strong>Export kopieren</strong>.</li>
        <li>Gib im Spiel <code>/wish import</code> ein.</li>
        <li>Füge mit <kbd>Strg</kbd>+<kbd>V</kbd> ein und bestätige.</li>
      </ol>
      <Shot name="18-wunschliste-export" caption="Der kopierte Text steht unter der Tabelle" />
      <Tip><strong>Noch kein Addon?</strong> Unter <Link to="/addons/raidwishlist">Addons → RaidWishlist</Link> kannst du es herunterladen. Dort steht auch, wie es installiert wird und was es kann.</Tip>
      <Tip><strong>Wichtig:</strong> Der Export enthält deinen aktuellen EP-Stand. Nach neuen EP oder Änderungen einfach neu kopieren und im Spiel neu importieren.</Tip>
    </Section>

    <Section id="ep" audience="mitglieder" title="EP, Item-Locks, Konto und Umfragen">
      <p>Dein Profil öffnest du mit einem Klick auf deinen Namen oben rechts. Dort gibt es die Reiter <strong>EP</strong>, <strong>Loot</strong>, <strong>Charaktere</strong>, <strong>Item-Locks</strong> und <strong>Konto</strong>.</p>
      <h3>EP (Enhance Points)</h3>
      <ul>
        <li>Wird ein Raid archiviert, bekommt jeder Teilnehmer <strong>ohne Main-Spec-Item +10 EP</strong>.</li>
        <li>Bei einem <strong>Enhanced</strong>-Wurf würfelst du <strong>1+X bis 100+X</strong>. X ist dein EP-Stand, höchstens 50.</li>
        <li><strong>Nur der Gewinner zahlt.</strong> Verlorene Würfe kosten nichts.</li>
      </ul>
      <p><strong>Beispiel:</strong> Sternfall hat 160 EP. Sein Enhanced-Wurf geht von <strong>51 bis 150</strong>, weil höchstens 50 EP zählen.</p>
      <Shot name="19-profil-ep" caption="Reiter EP: Stand, Wurfspanne und Historie" />
      <h3>Item-Locks</h3>
      <ul>
        <li>Mit einem Lock reservierst du ein Item in einem bestimmten Raid.</li>
        <li>Du brauchst <strong>mindestens 150 EP</strong>. Der Lock <strong>kostet nichts</strong>.</li>
        <li>Pro Raid kann nur eine Person dasselbe Item locken.</li>
        <li>Droppt es, bekommst du es automatisch als Main-Spec. Sonst verfällt der Lock am Raidende ohne Verlust.</li>
      </ul>
      <p>Reiter <strong>Item-Locks</strong> öffnen, Raid-ID, Item, Item-ID und Boss eintragen, <strong>Lock anlegen</strong>. Die Raid-ID ist die Zahl am Ende der Adresse eines Raids, z. B. <code>…/raids/2</code>.</p>
      <Shot name="20-item-locks" caption="Item-Locks: aktiver Lock und Formular" />
      <h3>Konto</h3>
      <p>Im Reiter <strong>Konto</strong> änderst du dein Passwort. Freiwillig kannst du eine Authenticator-App oder einen Sicherheitsschlüssel als zweiten Faktor einrichten. Warum sich das lohnt, steht unter <Link to="/kontosicherheit">Kontosicherheit</Link>.</p>
      <Shot name="21-konto" caption="Konto: Passwort und Zwei-Faktor" />
      <h3>Umfragen</h3>
      <p>Unter <Link to="/polls">Umfragen</Link> Antwort wählen und <strong>Abstimmen</strong> klicken.</p>
      <Shot name="22-umfragen" caption="Eine Umfrage" />
    </Section>

    <Section id="schattengram" audience="alle" title="Schattengram: Questreihe und Reihenfolge">
      <p>Unter <Link to="/schattengram">Schattengram</Link> steht für alle eingeloggten Gildenmitglieder, wer an Schattengram arbeitet, in welcher Reihenfolge und bei welchem Schritt der Questreihe: <strong>Vorquest, Seelen, Seuche, Blut, Frost, Splitter, Abschluss</strong>.</p>
      <p>Mehrere Leute können gleichzeitig dran sein. Seuche (Seuchenmord), Blut (Blutkönigin), Frost (Sindragosa) und die Splitter kann aber pro Raid nur eine Person machen. Dann hat der höhere Listenplatz Vorrang. Wer fertig ist, wandert in die Historie.</p>
      <Shot name="39-schattengram" caption="Schattengram: Liste mit Schritt und Splitterstand, darunter die Historie" />
      <p>Wer im einzelnen Raid welchen Schritt macht, steht im Raid unter <strong>Aufstellung</strong> im Kasten <strong>„Schattengram im Raid“</strong>. Vorgeschlagen wird der höchste Listenplatz aus der Aufstellung, der gerade bei diesem Schritt ist; die Raidleitung kann das pro Raid ändern. Zur Wahl stehen Charaktere von der Liste, die gerade bei diesem Schritt sind, und Krieger, Paladine und Todesritter aus der Aufstellung, die nicht auf der Liste stehen.</p>
      <Shot name="35b-schattengram-im-raid" caption="Schattengram im Raid: wer Seuche, Blut, Frost und Splitter macht" />
      <p><strong>Für die Raidleitung:</strong> Unter „Charakter eintragen“ kommen Krieger, Paladine und Todesritter auf die Liste, wahlweise direkt auf einen bestimmten Platz. Das Splitterziel ist 50 und lässt sich ändern, schon gesammelte Splitter trägst du gleich mit ein. Beim Schritt „Splitter“ zählst du mit <strong>+1 / −1</strong>, mit <strong>↑ / ↓</strong> oder „Bearbeiten“ änderst du Platz, Schritt und Stand. „Fertig“ geht, sobald alle Splitter gesammelt sind; „Entfernen“ nimmt einen falschen Eintrag wieder von der Liste.</p>
    </Section>

    <Section id="raids-anlegen" audience="raidleitung" title="Raids anlegen und vorausplanen">
      <p>Gilt für Raidassistenten, Raidleitung und Admins. Alles startet unter <Link to="/raids">Raids</Link>.</p>
      <h3>Einen Raid anlegen</h3>
      <ol>
        <li>Klicke auf <strong>Neuer Raid</strong>.</li>
        <li>Wähle die <strong>Raidserie</strong>, z. B. <em>ICC25 Donnerstag</em>. Raidleiter, Instanz, Größe und Name (z. B. <em>ICC25-6</em>) werden übernommen.</li>
        <li>Wähle den <strong>Start</strong> und klicke auf <strong>Anlegen</strong>.</li>
      </ol>
      <Shot name="30-raids-neuer-raid" caption="Neuer Raid aus einer Raidserie" />
      <h3>Mehrere Wochen im Voraus</h3>
      <ol>
        <li>Klicke auf <strong>Termine im Voraus anlegen</strong>.</li>
        <li>Wähle die <strong>Raidserie</strong> und bei <strong>Bis einschließlich</strong> das letzte Datum.</li>
        <li>Klicke auf <strong>Raids anlegen</strong>.</li>
      </ol>
      <p>Alle Termine bis dahin entstehen passend zu Wochentag und Uhrzeit. Vorhandene Termine bleiben unverändert. <strong>Die Anmeldung ist sofort offen.</strong></p>
      <p><strong>Beispiel:</strong> Serie <em>ICC25 Donnerstag</em> bis 12.11. ergibt ICC25-2 bis ICC25-5. Alle Mitglieder sehen sie sofort unter <em>Meine nächsten Raids</em>.</p>
      <Shot name="31-raids-voraus-planen" caption="Termine im Voraus anlegen und der Raidkalender" />
      <Tip>Beim Archivieren entsteht der nächste Termin der Serie automatisch. Admins können pro Serie einstellen, wie viele Wochen immer im Voraus geplant bleiben.</Tip>
      <h3>Verschieben oder absagen</h3>
      <p>Im Raid unter <strong>Planung &amp; Details</strong> den Start ändern und <strong>Planung speichern</strong>. Fällt der Raid aus, weiter unten <strong>Absagen</strong>.</p>
      <Shot name="38-planung-details" caption="Planung & Details" />
    </Section>

    <Section id="aufstellung" audience="raidleitung" title="Anmeldungen, Aufstellung und Gesuchtext">
      <h3>Anmeldungen steuern</h3>
      <ul>
        <li><strong>Anmeldungen schließen</strong> beendet die Anmeldung früher (und kann sie wieder öffnen).</li>
        <li><strong>Absagen &amp; Unsicher</strong> zeigt, wer nicht kann oder unsicher ist. <strong>Sicher dabei</strong> bzw. <strong>Absage zurücknehmen</strong> setzt jemanden wieder auf dabei.</li>
        <li>Unter <em>Spieler markieren</em> trägst du selbst jemanden als abgesagt oder unsicher ein, z. B. nach einer Discord-Nachricht.</li>
      </ul>
      <Shot name="32-raid-anmeldungen-leitung" caption="Anmeldungen aus Sicht der Raidleitung" />
      <h3>In die Aufstellung übernehmen</h3>
      <p>Unter <strong>Charakteroptionen nach Spieler</strong> steht jeder Angemeldete mit allen angebotenen Charakteren. Bei <em>Ausstehend</em> klickst du <strong>In Aufstellung übernehmen</strong>. Mit <strong>Auf diesen Charakter wechseln</strong> tauschst du den Charakter. Pro Spieler kommt nur ein Charakter in den Raid.</p>
      <Shot name="33-anmeldungen-uebernehmen" caption="Charakteroptionen nach Spieler" />
      <h3>Die Aufstellung</h3>
      <p>Der Reiter <strong>Aufstellung</strong> sortiert nach Tanks, Heilung, Nahkampf und Fernkampf, dazu die Ersatzbank. Mit <strong>Teilnahme hinzufügen</strong> trägst du jemanden ohne Anmeldung ein. Mit dem <strong>Stift</strong> änderst du Charakter und Spec oder nimmst jemanden mit <strong>Entfernen</strong> heraus.</p>
      <Shot name="34-aufstellung" caption="Raidaufstellung nach Rollen" />
      <Shot name="35-aufstellung-bearbeiten" caption="Teilnehmer bearbeiten oder entfernen" />
      <h3>Gesuchtext</h3>
      <ol>
        <li>Oben steht das Wunsch-Setup (ICC-Idealsetup vorbelegt). Pro Platz wählst du einen Spec, „Irgendein …“ oder „Leer“.</li>
        <li><strong>Noch gesuchte Positionen</strong> zeigt, was in der Aufstellung noch fehlt.</li>
        <li>Unter <strong>Textvorlage</strong> schreibst du deinen Text mit Platzhaltern und klickst <strong>Vorlage speichern</strong>.</li>
        <li>Mit <strong>Text kopieren</strong> holst du den fertigen Text und fügst ihn im Spiel ein.</li>
      </ol>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Platzhalter</th><th>Wird zu</th></tr></thead>
          <tbody>
            <tr><td><code>{'{{SETUP_KURZ}}'}</code></td><td>2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage, SP)</td></tr>
            <tr><td><code>{'{{TANK}}'}</code>, <code>{'{{HEAL}}'}</code>, <code>{'{{MDD}}'}</code>, <code>{'{{RDD}}'}</code></td><td>Nur die Anzahl, z. B. 2</td></tr>
            <tr><td><code>{'{{HEAL_SPECS}}'}</code> (auch TANK, MDD, RDD)</td><td>Nur die Specs, z. B. Baum, Diszi</td></tr>
            <tr><td><code>{'{{SETUP}}'}</code></td><td>Alle gesuchten Plätze ausgeschrieben</td></tr>
          </tbody>
        </table>
      </div>
      <p><strong>Beispiel:</strong> <code>{'LFM ICC25 Donnerstag 20 Uhr – suchen noch {{SETUP_KURZ}}. /w Thorgrim'}</code> wird zu <em>LFM ICC25 Donnerstag 20 Uhr – suchen noch 1 Tank (OT), 4 Heal (Resto Schami, Baum, Diszi, beliebig), … /w Thorgrim</em>.</p>
      <Shot name="36-gesuchtext" caption="Gesuchtext: gesuchte Plätze, Vorlage, fertiger Text" />
    </Section>

    <Section id="loot" audience="raidleitung" title="Loot eintragen und archivieren">
      <h3>Loot eintragen</h3>
      <p>Gewürfelt wird im Spiel. Hier wird nur eingetragen, wer was bekommen hat: Reiter <strong>Loot</strong>, <strong>Loot erfassen</strong>, Boss und Modus wählen, Item suchen, Empfänger und Vergabeart wählen.</p>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Vergabeart</th><th>Bedeutung</th><th>+10 EP beim Archivieren?</th></tr></thead>
          <tbody>
            <tr><td><strong>Main-Spec</strong></td><td>Für den Haupt-Spec</td><td>Nein</td></tr>
            <tr><td><strong>Enhanced</strong></td><td>Mit EP-Wurf gewonnen, EP werden automatisch abgezogen</td><td>Nein</td></tr>
            <tr><td><strong>Second-Spec</strong></td><td>Für den zweiten Spec</td><td>Ja</td></tr>
            <tr><td><strong>BoE</strong></td><td>Spieler-BoE</td><td>Ja</td></tr>
          </tbody>
        </table>
      </div>
      <Shot name="37-loot" caption="Loot erfassen" />
      <p><strong>Schneller mit dem Addon:</strong> In RaidLedger auf <strong>EXPORT</strong> klicken, den Text unter <em>Loot aus Addon importieren</em> einfügen, <strong>Vorschau</strong>, jede Zeile prüfen und übernehmen. Download und alle Funktionen stehen unter <Link to="/addons/raidledger">Addons → RaidLedger</Link>.</p>
      <h3>Beute ohne Empfänger</h3>
      <p>Items, die entzaubert wurden, in die Gildenbank gehen oder die niemand wollte, gehören keinem Spieler. Sie stehen im Reiter <strong>Loot</strong> unter <strong>Beute ohne Empfänger</strong>: aufklappen, Boss (optional), Item, Verbleib und bei wem es liegt eintragen. Sie zählen für niemanden und kosten keine EP. Der Addon-Import übernimmt Zeilen, die RaidLedger als <em>DE</em> oder <em>BANK</em> vergeben hat, automatisch dorthin.</p>
      <h3>Archivieren nach dem Raid</h3>
      <p>Archivieren beendet den Raid endgültig. Teilnehmer ohne Main-Spec-Loot bekommen +10 EP, ungenutzte Locks verfallen, und der nächste Serientermin entsteht.</p>
      <ol>
        <li>Reiter <strong>Planung &amp; Details</strong> öffnen und nach unten scrollen.</li>
        <li>Die <strong>EP-Übersicht vor dem Archivieren</strong> prüfen.</li>
        <li>Abgesagte bekommen keine EP. Soll jemand trotzdem +10 EP bekommen, Häkchen setzen.</li>
        <li><strong>Jetzt archivieren</strong> klicken.</li>
      </ol>
      <p><strong>Beispiel:</strong> Thorgrim hat 10 EP und bekommt kein Main-Spec-Item. Danach hat er 20 EP.</p>
      <Shot name="38b-archivieren-pruefen" caption="Prüfen vor dem Archivieren" />
      <p>Auch archivierte Raids lassen sich korrigieren, z. B. mit <strong>Loot korrigieren</strong>. Dann ist immer eine Begründung nötig, und EP werden automatisch ausgeglichen.</p>
      <Shot name="39-archivierter-raid-loot" caption="Archivierter Raid: Loot korrigieren" />
    </Section>

    <Section id="admins" audience="admins" title="Spieler, Registrierung, Raidserien, Protokoll">
      <h3>Neue Leute freischalten</h3>
      <ol>
        <li>Die Person fordert auf der Login-Seite einen Code an.</li>
        <li>Unter <strong>Registrierung</strong> erscheint die Anfrage. Erstelle einen Code. Er wird nur einmal angezeigt.</li>
        <li>Schick den Code ingame an genau diesen Charakter und markiere ihn als gesendet.</li>
        <li>Die Person registriert sich. Gibt es den Charakter schon (z. B. aus der alten Liste), bekommt <strong>der Spieler, dem der Charakter gehört</strong>, den Login. Sonst entsteht ein neuer Spieler als <strong>Gast</strong>.</li>
        <li>Mach die Person zum <strong>Mitglied</strong>, falls sie es noch nicht ist (nächster Schritt).</li>
      </ol>
      <p>Im Reiter <strong>Verlauf</strong> zeigt die Spalte <strong>Verknüpfter Spieler</strong>, für welchen Spieler ein Code eingelöst wurde, mit Login-Namen und Datum.</p>
      <p><strong>Beispiel:</strong> Die Anfrage für den Charakter <em>Mirabelle</em> ist eingelöst und mit dem Spieler <em>Mira</em> (Login <code>@mira</code>) verknüpft.</p>
      <Shot name="53-registrierung" caption="Verlauf: welcher Code welchem Spieler zugeordnet wurde" />
      <h3>Wer hat schon ein Konto?</h3>
      <p>Unter <strong>Spieler</strong> sehen Admins zu jedem Spieler seine <strong>Charaktere</strong>, das <strong>Konto</strong> und die <strong>Rolle</strong>. Oben steht, wie viele Spieler schon ein Konto haben. Wurde ein Charakter versehentlich dem falschen Spieler zugeordnet, öffne beim bisherigen Spieler den Reiter „Charaktere“, klicke beim Charakter auf <strong>Verschieben</strong> und wähle den richtigen Spieler. Die Raid-Aufstellungen zeigen danach den richtigen Spieler; Loot und EP bleiben unverändert.</p>
      <Shot name="52b-charakter-verschieben" caption="Charakter dem richtigen Spieler zuordnen" />
      <ul>
        <li><strong>@login</strong> mit „registriert am … über ‹Charakter›“: Die Person hat einen Code eingelöst.</li>
        <li><strong>@login</strong> mit „Login vom Admin angelegt“: Ein Admin hat Benutzername und Passwort gesetzt.</li>
        <li><span className="badge warn">Kein Konto</span>: Diesen Spieler hat noch niemand übernommen.</li>
      </ul>
      <p>Mit dem Filter <strong>Konto</strong> zeigst du nur Spieler <em>mit</em> oder <em>ohne</em> Konto. Die Suche findet auch Charakter- und Login-Namen.</p>
      <Shot name="51-spieler" caption="Spielerliste für Admins: Charaktere, Konto und Rolle" />
      <p><strong>Beispiel:</strong> Mit „Ohne Konto“ siehst du, dass <em>Altes Eisen</em> (Charaktere Klingensturm und Funkelbart) und <em>Grimmbart</em> sich noch nicht registriert haben. So weißt du, wen du ansprechen musst.</p>
      <Shot name="51b-spieler-ohne-konto" caption="Nur Spieler ohne Konto" />
      <h3>Rolle ändern, Passwort zurücksetzen</h3>
      <p>Unter <strong>Spieler</strong> auf den Namen klicken, Reiter <strong>Konto</strong>. Dort Rolle wählen (z. B. <code>MEMBER</code>), bei Bedarf ein neues Passwort setzen, <strong>Speichern</strong>.</p>
      <p><strong>Beispiel:</strong> Ein frisch registrierter Spieler ist <code>GUEST</code>. Du stellst <code>MEMBER</code> ein und speicherst. Ab sofort kann er sich für Raids anmelden.</p>
      <Shot name="52-spieler-konto-admin" caption="Spielerkonto als Admin" />
      <p>Hat jemand zwei Accounts, nutze <strong>Konto zusammenführen</strong> mit einem Grund. Unter <strong>Spieler</strong> → <strong>Neuer Spieler</strong> legst du jemanden ohne Login an.</p>
      <h3>Raidserien</h3>
      <p>Eine Raidserie ist ein fester Raid wie <em>ICC25 Donnerstag</em>: Wochentag, Uhrzeit, Rhythmus, Namensvorlage (<code>{'ICC25-{number}'}</code>), Raidleitung, Instanz und Größe. Dort stellst du auch ein, wie viele Wochen automatisch im Voraus geplant werden.</p>
      <Shot name="50-raidserien" caption="Raidserien" />
      <h3>Boss-Reihenfolge</h3>
      <p>Unten auf der Seite <strong>Beute</strong> stellst du pro Instanz die Reihenfolge der Bosse ein: mit <strong>↑ / ↓</strong> verschieben, dann <strong>Reihenfolge speichern</strong>. Sie gilt für die Boss-Auswahl beim Loot-Eintragen und für die Boss-Gruppen in der Beuteliste eines Raids.</p>
      <h3>Gildenkonfiguration</h3>
      <p>Gildenränge und Raidgruppen pflegen. <em>deaktivieren</em> blendet aus, ohne zu löschen.</p>
      <Shot name="54-gildenkonfiguration" caption="Gildenkonfiguration" />
      <h3>Audit-Protokoll</h3>
      <p>Jede Änderung mit Zeitpunkt, wer es war, was passiert ist und Grund. Über das Suchfeld findest du alles zu einem Spieler.</p>
      <p><strong>Beispiel:</strong> Sternfall fragt, warum er nicht mehr angemeldet ist. Im Protokoll siehst du, ob er selbst abgesagt hat oder ob ihn jemand entfernt hat.</p>
      <Shot name="55-audit-protokoll" caption="Audit-Protokoll" />
    </Section>

    <Section id="fragen" audience="alle" title="Häufige Fragen">
      <dl className="guide-faq">
        <dt>Ich bin registriert, kann mich aber nicht anmelden.</dt>
        <dd>Du bist wahrscheinlich noch Gast (oben rechts steht <code>GUEST</code>). Bitte einen Admin, dich zum Mitglied zu machen.</dd>
        <dt>Ich habe mich angemeldet. Bin ich fix dabei?</dt>
        <dd>Erst wenn auf deiner Karte <strong>Bestätigt</strong> steht. <em>Angemeldet</em> heißt nur: Die Raidleitung weiß Bescheid.</dd>
        <dt>Ich habe aus Versehen „Kann nicht“ geklickt.</dt>
        <dd>Klick auf <strong>Sicher dabei</strong>, solange die Anmeldung offen ist. Sonst sag der Raidleitung Bescheid.</dd>
        <dt>Wo ist der Export für das Wunschlisten-Addon?</dt>
        <dd><Link to="/wishlist">Meine Wunschliste</Link> → <strong>Export kopieren</strong> → im Spiel <code>/wish import</code> → <kbd>Strg</kbd>+<kbd>V</kbd>.</dd>
        <dt>Mein EP-Stand im Addon ist alt.</dt>
        <dd>Export neu kopieren und neu importieren.</dd>
        <dt>Ich kann keinen Item-Lock anlegen.</dt>
        <dd>Du brauchst mindestens 150 EP, und niemand anderes darf das Item im selben Raid gelockt haben.</dd>
        <dt>Warum habe ich keine +10 EP bekommen?</dt>
        <dd>Wahrscheinlich hast du ein Main-Spec- oder Enhanced-Item bekommen. Schau in deinem Profil unter EP in die Historie.</dd>
        <dt>Passwort vergessen?</dt>
        <dd>Ein Admin setzt dir ein neues. Danach änderst du es selbst im Profil unter Konto.</dd>
      </dl>
    </Section>
  </div>
}
