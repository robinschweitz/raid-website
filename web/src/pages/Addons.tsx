import { useEffect, useState, type ReactNode } from 'react'
import { api } from '../api'
import { NavIcon } from '../components/NavIcon'
import { Link } from '../router'
import type { AddonDownload } from '../types'

// The addons pages (ADR 085): a showcase per addon with downloads served by the
// website itself. Screenshots in public/addons are cropped from live test runs
// on the local test realm (addon-forge, examples/.forge-runs).

type AddonID = 'raidledger' | 'raidwishlist'

const addonNames: Record<AddonID, string> = { raidledger: 'RaidLedger', raidwishlist: 'RaidWishlist' }

function useAddons() {
  const [addons, setAddons] = useState<AddonDownload[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    api.get<AddonDownload[]>('/api/v1/addons')
      .then((list) => { if (active) setAddons(list) })
      .catch(() => { if (active) setError('Die Downloads konnten nicht geladen werden. Bitte die Seite neu laden.') })
    return () => { active = false }
  }, [])
  return { addons, error }
}

function sizeLabel(bytes: number) {
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

function DownloadButton({ addon }: { addon?: AddonDownload }) {
  if (!addon) return <span className="button-link primary addon-download disabled" aria-disabled="true"><NavIcon name="download" />Download lädt…</span>
  return <a className="button-link primary addon-download" href={addon.download_url} download={addon.file_name}>
    <NavIcon name="download" />{addon.name} {addon.version} herunterladen <small>ZIP, {sizeLabel(addon.size_bytes)}</small>
  </a>
}

function Shot({ name, caption, width, height }: { name: string, caption: string, width: number, height: number }) {
  const src = `/addons/${name}.webp`
  return <figure className="addon-shot">
    <a href={src} target="_blank" rel="noreferrer" title="Bild in voller Größe öffnen">
      <img src={src} alt={caption} loading="lazy" width={width} height={height} />
    </a>
    <figcaption>{caption}</figcaption>
  </figure>
}

function Feature({ title, children, shot }: { title: string, children: ReactNode, shot?: ReactNode }) {
  return <article className={shot ? 'addon-feature' : 'addon-feature no-shot'}>
    <div className="addon-feature-text">
      <h3>{title}</h3>
      {children}
    </div>
    {shot && <div className="addon-feature-shot">{shot}</div>}
  </article>
}

function Code({ children }: { children: string }) {
  return <pre className="addon-code"><code>{children}</code></pre>
}

function Tip({ children }: { children: ReactNode }) {
  return <p className="guide-tip">{children}</p>
}

function InstallSteps({ name }: { name: string }) {
  return <>
    <ol className="addon-steps">
      <li><strong>Herunterladen</strong> und die ZIP-Datei öffnen. Darin liegt genau ein Ordner: <code>{name}</code>.</li>
      <li>Diesen Ordner nach <code>World of Warcraft\Interface\AddOns\</code> entpacken. Fehlt der Ordner <code>AddOns</code>, leg ihn an.</li>
      <li>Prüfen: Die Datei <code>{`Interface\\AddOns\\${name}\\${name}.toc`}</code> muss direkt in diesem Ordner liegen, nicht in einem zweiten Unterordner.</li>
      <li>WoW starten (oder neu starten), in der Charakterauswahl unten links auf <strong>AddOns</strong> klicken und <strong>{name}</strong> anhaken.</li>
    </ol>
    <Code>{`World of Warcraft\\
└── Interface\\
    └── AddOns\\
        └── ${name}\\
            ├── ${name}.toc
            └── … (.lua-Dateien)`}</Code>
    <Tip><strong>Update:</strong> Neue Version herunterladen, den alten Ordner <code>{name}</code> löschen und den neuen entpacken. Deine Daten (Vergaben, Wunschliste, Einstellungen) liegen in <code>WTF\…\SavedVariables</code> und bleiben erhalten.</Tip>
  </>
}

function AddonHero({ addon, eyebrow, pitch, audience, shot }: { addon?: AddonDownload, eyebrow: string, pitch: string, audience: string, shot: ReactNode }) {
  return <section className="card addon-hero">
    <div className="addon-hero-text">
      <p className="eyebrow">{eyebrow}</p>
      <h1>{addon?.name ?? '…'}</h1>
      <p className="addon-pitch">{pitch}</p>
      <p className="addon-meta">
        <span className="badge">Version {addon?.version ?? '…'}</span>
        <span className="badge">WoW 3.3.5a</span>
        <span className="badge">{audience}</span>
      </p>
      <DownloadButton addon={addon} />
      <p className="muted addon-hero-note">Spricht Deutsch im deutschen Client. <Link to="/addons">Alle Addons</Link></p>
    </div>
    <div className="addon-hero-shot">{shot}</div>
  </section>
}

function Contents({ entries }: { entries: { id: string, title: string }[] }) {
  return <nav className="addon-contents" aria-label="Inhalt">
    {entries.map((entry) => <button type="button" key={entry.id} className="link-button" onClick={() => document.getElementById(entry.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>{entry.title}</button>)}
  </nav>
}

function Overview({ addons, error }: { addons: AddonDownload[], error: string }) {
  const find = (name: string) => addons.find((addon) => addon.name === name)
  return <>
    <div className="page-heading">
      <div>
        <p className="eyebrow">WoW 3.3.5a</p>
        <h1>Addons</h1>
      </div>
    </div>
    <p className="guide-lead">Zwei Addons verbinden das Spiel mit dieser Website: Die Raidleitung lädt mit <strong>RaidLedger</strong> ein und verteilt den Loot, jeder Raider würfelt mit <strong>RaidWishlist</strong> auf seine Wunschliste. Alles läuft über Kopieren und Einfügen. Die Addons verbinden sich nie selbst mit dem Internet.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="addon-cards">
      <article className="card addon-card ledger">
        <div className="addon-card-shot"><img src="/addons/raidledger-fenster.webp" alt="Das RaidLedger-Fenster im Spiel" width={709} height={448} /></div>
        <div className="addon-card-body">
          <p className="eyebrow">Für Raidleitung &amp; Plündermeister</p>
          <h2>RaidLedger</h2>
          <p>Lädt die Aufstellung von der Website ein, legt den Kalender-Termin an, würfelt den Loot nach unseren Regeln aus und schickt Teilnahme und Loot zurück an die Website.</p>
          <div className="actions">
            <DownloadButton addon={find('RaidLedger')} />
            <Link className="button-link" to="/addons/raidledger">So funktioniert es &rarr;</Link>
          </div>
        </div>
      </article>
      <article className="card addon-card wishlist">
        <div className="addon-card-shot"><img src="/addons/raidwishlist-fenster.webp" alt="Das RaidWishlist-Fenster im Spiel" width={466} height={409} /></div>
        <div className="addon-card-body">
          <p className="eyebrow">Für jeden Raider</p>
          <h2>RaidWishlist</h2>
          <p>Holt deine Wunschliste von der Website ins Spiel. Wird ein Item davon ausgewürfelt, würfelst du mit einem Klick richtig (oder automatisch) und siehst, wer was bekommen hat.</p>
          <div className="actions">
            <DownloadButton addon={find('RaidWishlist')} />
            <Link className="button-link" to="/addons/raidwishlist">So funktioniert es &rarr;</Link>
          </div>
        </div>
      </article>
    </div>

    <section className="card guide-section" aria-labelledby="ablauf-title">
      <h2 id="ablauf-title">Ein Raidabend mit beiden Addons</h2>
      <ol className="addon-flow">
        <li><strong>Website:</strong> Du setzt in <Link to="/wishlist">Meine Wunschliste</Link> deine Items, kopierst den Export und holst ihn mit <code>/wish import</code> ins Spiel.</li>
        <li><strong>Website → RaidLedger:</strong> Die Raidleitung kopiert im Raid unter <em>Aufstellung</em> die „Einladungen für Addon“, liest sie ein, legt den Kalender-Termin an und lädt alle ein. Anmeldungen, „unsicher“ und Absagen der Website stehen mit ihrem Status im Spielkalender.</li>
        <li><strong>Spielkalender → Website:</strong> Wer im Spiel auf die Kalender-Einladung antwortet, kommt mit „ANTWORTEN FÜR WEBSEITE“ zurück auf die Website: Zusage wird zur Anmeldung, Absage zu „kann nicht“.</li>
        <li><strong>Im Raid:</strong> RaidLedger startet die Würfe. RaidWishlist meldet sich bei deinen Wunsch-Items und würfelt den richtigen Bereich, auch mit Enhanced.</li>
        <li><strong>RaidLedger → Website:</strong> Nach dem Raid kopiert die Raidleitung Teilnahme und Loot aus RaidLedger und importiert beides im Raid auf der Website.</li>
      </ol>
    </section>

    <section className="card guide-section" id="installation" aria-labelledby="installation-title">
      <h2 id="installation-title">Installation</h2>
      <p>Raider brauchen nur <strong>RaidWishlist</strong>, die Raidleitung und der Plündermeister <strong>RaidLedger</strong> (gern zusätzlich RaidWishlist). Hier am Beispiel RaidWishlist, für RaidLedger geht es genauso mit dem Ordner <code>RaidLedger</code>.</p>
      <InstallSteps name="RaidWishlist" />
      <Tip><strong>Noch RaidSync installiert?</strong> RaidLedger ersetzt RaidSync komplett. Lösch den alten Ordner <code>Interface\AddOns\RaidSync</code>.</Tip>
    </section>
  </>
}

function RaidLedgerPage({ addon, error }: { addon?: AddonDownload, error: string }) {
  return <>
    <AddonHero
      addon={addon}
      eyebrow="Für Raidleitung & Plündermeister"
      audience="Raidleitung"
      pitch="Das Werkzeug für den Raidabend: Aufstellung von der Website einladen, Kalender-Termin anlegen, Loot nach unseren Regeln auswürfeln, vergeben, nachhalten und am Ende Teilnahme und Loot zurück auf die Website bringen."
      shot={<Shot name="raidledger-fenster" caption="RaidLedger, Reiter BEUTE: das Item, seine Würfe mit BISHER und der Gewinner" width={709} height={448} />}
    />
    {error && <p className="error" role="alert">{error}</p>}
    <Contents entries={[
      { id: 'ledger-funktionen', title: 'Funktionen' },
      { id: 'ledger-website', title: 'Zusammenspiel mit der Website' },
      { id: 'ledger-befehle', title: 'Befehle' },
      { id: 'ledger-installation', title: 'Installation' },
    ]} />

    <section className="card guide-section" id="ledger-funktionen" aria-labelledby="ledger-funktionen-title">
      <h2 id="ledger-funktionen-title">Funktionen</h2>
      <p>Öffnen mit <code>/ledger</code> oder dem Minimap-Knopf (zum Verschieben ziehen, <code>/ledger minimap off</code> blendet ihn aus). Das Fenster hat fünf Reiter, jeder mit der vollen Höhe, damit auch lange Beutelisten lesbar bleiben. Alles geht per Klick, jeder Knopf erklärt sich mit einem Tooltip; Chat-Befehle braucht man nicht mehr.</p>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Reiter</th><th>Wofür</th></tr></thead>
          <tbody>
            <tr><td><strong>BEUTE</strong></td><td>Die Beute dieser Leiche, darunter die offenen Items, und das ausgewählte Item: würfeln, Würfe, Empfänger, vergeben</td></tr>
            <tr><td><strong>VERGABEN</strong></td><td>Jede Vergabe dieses Raids: Empfänger ändern, Main-/Second-Spec, entfernen, Würfe; Export und neuer Raid</td></tr>
            <tr><td><strong>SPIELER</strong></td><td>Items und Tokens pro Spieler und Spec, mit Korrekturen per − / +</td></tr>
            <tr><td><strong>RAID</strong></td><td>Einladen, Gruppen, Teilnahme, Kalender-Termin</td></tr>
            <tr><td><strong>EINSTELLUNGEN</strong></td><td>Wer ändern darf, wer welches Addon hat, Wurfzeiten, Minimap-Knopf, Fenster, Abgleich</td></tr>
          </tbody>
        </table>
      </div>

      <Feature title="1. Raid aufstellen" shot={<Shot name="raidledger-aufstellung" caption="RAID: Einladungsliste mit Datum und Uhrzeit eingelesen" width={709} height={448} />}>
        <ol>
          <li>Auf der Website im Raid unter <strong>Aufstellung</strong> bei <em>Einladungen für Addon</em> auf <strong>Für Addon kopieren</strong> klicken.</li>
          <li>Im Spiel den Reiter <strong>RAID</strong> öffnen (oder <code>/ledger setup</code>), mit <kbd>Strg</kbd>+<kbd>V</kbd> einfügen und <strong>EINLESEN</strong>.</li>
          <li><strong>ALLE EINLADEN</strong> lädt in der Reihenfolge der Website ein. Offline-Gildenmitglieder werden übersprungen, die Ersatzbank nur mit <strong>ERSATZBANK: JA</strong>.</li>
          <li><strong>FEHLENDE ERNEUT</strong> lädt nach, wer noch fehlt. <strong>GRUPPEN SORTIEREN</strong> stellt Tanks, Heiler und den Rest in Fünfergruppen.</li>
        </ol>
        <p>Jeder Charakter steht mit Rolle und Status in der Liste: offen, eingeladen, im Raid, abgelehnt, offline, keine Antwort.</p>
      </Feature>

      <Feature title="2. Kalender-Termin anlegen" shot={<>
        <Shot name="raidledger-termin-erstellen" caption="Nach KALENDER-TERMIN: TERMIN ERSTELLEN legt den Termin an" width={709} height={448} />
        <Shot name="raidledger-termin-chat" caption="Der Chat meldet, dass der Server den Termin bestätigt hat" width={410} height={68} />
      </>}>
        <p>Die Einladungsliste enthält <strong>Datum und Startzeit</strong> des Raids. Daraus legt RaidLedger einen Raid-Termin im Spielkalender an und lädt alle ein, auch die Ersatzbank.</p>
        <ol>
          <li>Liste einlesen (siehe oben), dann <strong>KALENDER-TERMIN</strong> klicken (oder <code>/ledger calendar</code>). RaidLedger lädt etwa alle 2 Sekunden einen Charakter ein (schneller lässt das Spiel nicht zu); der Knopf zeigt <strong>LADE EIN 3/25…</strong>.</li>
          <li>Der Knopf heißt jetzt <strong>TERMIN ERSTELLEN (n)</strong>. Klicken, und der Termin entsteht mit allen Einladungen.</li>
          <li>RaidLedger liest den Kalender danach zurück und meldet im Chat, ob der Server den Termin bestätigt hat. Unbekannte Namen werden genannt.</li>
        </ol>
        <Tip><strong>Gut zu wissen:</strong> Datum und Uhrzeit stehen nur bei Raids aus einer <strong>Raidserie</strong> in der Liste, in der Zeitzone der Serie (Serverzeit). Der Spielkalender nimmt nur Termine in der Zukunft und Titel bis 100 Zeichen.</Tip>
      </Feature>

      <Feature title="2b. Spielkalender mit der Website abgleichen">
        <p>Die Einladungsliste enthält nicht nur die Aufstellung: Wer auf der Website <strong>angemeldet</strong> ist, <strong>unsicher</strong> ist oder <strong>abgesagt</strong> hat und noch keinen Platz in der Aufstellung hat, steht mit dieser Antwort darunter. Diese Charaktere kommen nur in den Kalender-Termin, nie in die Raidgruppe.</p>
        <ol>
          <li><strong>ABGLEICHEN</strong> (rechts unter <em>SPIELKALENDER</em>, oder <code>/ledger calendar sync</code>) öffnet den bestehenden Termin und lädt Fehlende ein; nach TERMIN ERSTELLEN läuft das automatisch. Danach heißt der Knopf <strong>STATUS SETZEN (n)</strong>: Ein Klick setzt die Status (Aufstellung <em>Bestätigt</em>, Ersatzbank <em>Ersatzbank</em>, Anmeldung <em>Zugesagt</em>, Absage <em>Abgelehnt</em>) und meldet kurz danach, wie viele davon im Kalender angekommen sind.</li>
          <li>Eine Antwort, die ein Spieler selbst im Kalender gegeben hat, wird <strong>nie überschrieben</strong>. Sagt jemand aus der Aufstellung im Spiel ab, nennt RaidLedger ihn im Chat.</li>
          <li><strong>ANTWORTEN FÜR WEBSEITE</strong> (<code>/ledger calendar export</code>) liest alle Antworten des Termins aus. Text kopieren und im Raid unter <em>Aufstellung</em> → <em>Antworten aus dem Spielkalender</em> einfügen: Die Website zeigt eine Vorschau, Zusagen werden zu offenen Anmeldungen mit der Main-Spec, Absagen zu „kann nicht“.</li>
        </ol>
        <Tip><strong>Gut zu wissen:</strong> Zum Ändern eines Termins muss man ihn erstellt haben oder Moderator sein. Der 3.3.5-Kalender kennt kein „Vorläufig“: Wer auf der Website unsicher ist, bleibt im Kalender „Eingeladen“.</Tip>
      </Feature>

      <Feature title="3. Würfeln nach unseren Regeln" shot={<Shot name="raidledger-wurf-chat" caption="Ansagen im Gruppenchat: Wurfstart, Countdown, Gewinner mit bisherigen Items" width={380} height={116} />}>
        <p>Im Reiter <strong>BEUTE</strong> das Item auswählen und <strong>WÜRFELN</strong> klicken (oder <code>/ledger start</code>): Der Raid hat 30 Sekunden, einstellbar unter <strong>EINSTELLUNGEN</strong>. Zur Hälfte und zu einem Viertel der Zeit erinnert das Addon.</p>
        <ul>
          <li><code>/rnd 100</code> ist <strong>Main-Spec</strong>, <code>/rnd 99</code> ist <strong>Second-Spec</strong>.</li>
          <li><strong>Enhanced:</strong> Main-Spec mit <strong>1+X bis 100+X</strong>, X = EP, höchstens 50. Die EP zieht die Website beim Loot-Import ab.</li>
          <li>Main-Spec schlägt immer Second-Spec. Innerhalb eines Specs gewinnt, <strong>wer in diesem Raid weniger Items hat</strong>, dann der Wurf. Tokens (Weiheabzeichen) zählen getrennt.</li>
          <li>Nur der erste Wurf zählt. Bei Gleichstand gibt es 15 Sekunden Stechen.</li>
          <li>Gleiche Tokens von einer Leiche sind <strong>eine Zeile „3x“</strong> und ein Wurf: Die besten drei gewinnen.</li>
        </ul>
        <p>Neben der Beute steht die Wurfliste in Gewinner-Reihenfolge, mit <strong>BISHER</strong>: wie viele Items der Würfler in diesem Spec schon hat. <strong>Rechtsklick</strong> auf einen Wurf: <strong>PASST</strong> (der Nächste gewinnt), als <strong>MAIN-SPEC</strong>, <strong>SECOND-SPEC</strong>, <strong>ZÄHLT NICHT</strong> oder mit korrigierten Enhanced-Punkten werten. Jede Änderung wird im Raid angesagt.</p>
      </Feature>

      <Feature title="4. Vergeben und nachhalten" shot={<Shot name="raidledger-vergaben" caption="VERGABEN: die ausgewählte Vergabe mit Spec, Empfänger und Würfen" width={709} height={448} />}>
        <ul>
          <li>Nach dem Wurf steht der Gewinner im Feld <strong>EMPFÄNGER</strong>. Klick auf einen Wurf macht diesen Spieler zum Empfänger, <strong>ALLE (n)</strong> zeigt den ganzen Raid nach Gruppen.</li>
          <li><strong>GEBEN + ERFASSEN</strong> gibt das Item per Plündermeister und trägt die Vergabe ein. <strong>DE</strong> und <strong>BANK</strong> geben ungewollte Items an den Verzauberer oder die Gildenbank. Sie zählen für niemanden.</li>
          <li>Items, die du selbst behältst, folgt das Addon durch deine Handel bis zum Gewinner.</li>
          <li>Im Reiter <strong>VERGABEN</strong> eine Vergabe auswählen: <strong>ZÄHLT ALS</strong> Main-Spec, Second-Spec oder nichts, <strong>EMPFÄNGER ÄNDERN</strong>, parken, DE, BANK oder <strong>ENTFERNEN</strong>. Darunter stehen ihre Würfe.</li>
          <li>Beim <strong>Plündern als Gruppe</strong> trägt RaidLedger die Gewinner („X gewinnt: …“) ohne Spec ein. Sie zählen erst, wenn du einen Spec setzt.</li>
        </ul>
        <p>Alle RaidLedger-Nutzer in der Gruppe sehen dieselbe Liste; angesagt wird jede Vergabe nur von dem Ledger, der sie eingetragen hat. Nach einem Absturz holt <strong>VERGABEN ABGLEICHEN</strong> (oder <code>/ledger sync</code>) die Vergaben aus der Gruppe zurück.</p>
      </Feature>

      <Feature title="5. Items pro Spieler: SPIELER" shot={<Shot name="raidledger-spieler" caption="SPIELER: Items und Tokens pro Spieler, nach Main- und Second-Spec" width={709} height={448} />}>
        <p>Der Reiter <strong>SPIELER</strong> zeigt jeden im Raid mit seinen Items und Tokens, je Main- und Second-Spec: genau die Zahlen, nach denen „wer weniger hat, gewinnt“ entscheidet.</p>
        <ul>
          <li>Spieler auswählen: <strong>−</strong> / <strong>+</strong> korrigiert die Zahlen, z. B. für Loot von vor dem Start des Addons. Korrigierte Werte stehen gold mit <strong>*</strong>.</li>
          <li>Korrekturen zählen beim Würfeln, gehen an die anderen Ledger in der Gruppe, werden mit <strong>NEUER RAID</strong> zurückgesetzt und kommen nie in den Website-Export.</li>
          <li>Darunter stehen die Vergaben des Spielers; ein Klick öffnet sie in VERGABEN.</li>
        </ul>
      </Feature>

      <Feature title="6. Später würfeln: PARKEN, Taschen, offene Items" shot={<Shot name="raidledger-offene-items" caption="Ein offenes Item bleibt in der Beute unter OFFENE ITEMS, bei Test" width={709} height={448} />}>
        <ul>
          <li><strong>PARKEN</strong> gibt ein Item einem Raider (oder dir), der es hält, bis gewürfelt wird, z. B. bei einzigartigen Items.</li>
          <li><strong>Items aus deinen Taschen</strong> würfelst du mit <kbd>Alt</kbd>+Klick auf das Item aus (oder <code>/ledger roll</code>). Der Gewinner bekommt es per Handel.</li>
          <li>Offene Items bleiben im Reiter <strong>BEUTE</strong> unter <strong>OFFENE ITEMS</strong>, mit dem, der sie hat, bis sie ausgewürfelt sind. So geht nichts vergessen. Auswählen und <strong>WÜRFELN</strong>.</li>
        </ul>
      </Feature>

      <Feature title="7. Wer darf ändern? EINSTELLUNGEN">
        <ul>
          <li><strong>Bearbeiter:</strong> Nur der Plündermeister und die Namen auf dieser Liste ändern Würfe und Vergaben. Alle anderen RaidLedger schauen nur zu. Die Liste geht an die ganze Gruppe (auch <code>/ledger allow</code> / <code>/ledger deny</code>).</li>
          <li><strong>Wer hat welches Addon:</strong> PRÜFEN fragt die Gruppe, wer RaidLedger oder RaidWishlist in welcher Version hat.</li>
          <li>Dazu Wurfzeit und Stechen, Minimap-Knopf, Fenster zentrieren und <strong>VERGABEN ABGLEICHEN</strong>.</li>
        </ul>
      </Feature>

      <Feature title="8. Wer will was? Die Wunschlisten im Blick">
        <p>Raider mit RaidWishlist teilen ihre Wunschliste mit deinem RaidLedger. Jeder Drop zeigt <strong>WL n</strong>: wie viele Wunschlisten ihn nennen. Mit der Maus darüber siehst du, wer ihn will (Main-Spec zuerst, nach Rang, mit Enhanced). In der Wurfliste steht bei Würflern mit dem Item auf der Liste <code>[WL #Rang]</code>.</p>
      </Feature>
    </section>

    <section className="card guide-section" id="ledger-website" aria-labelledby="ledger-website-title">
      <h2 id="ledger-website-title">Zusammenspiel mit der Website</h2>
      <p>Drei Texte wandern zwischen Website und Spiel, alle mit <code>;</code> getrennt.</p>
      <div className="table-scroll">
        <table className="table">
          <thead><tr><th>Text</th><th>Richtung</th><th>Wo auf der Website</th><th>Wo im Addon</th></tr></thead>
          <tbody>
            <tr><td><code>RS2;INV</code></td><td>Website → Spiel</td><td>Raid → Aufstellung → <em>Einladungen für Addon</em> → <strong>Für Addon kopieren</strong></td><td>Reiter <strong>RAID</strong> → einfügen → <strong>EINLESEN</strong></td></tr>
            <tr><td><code>RS2;ROSTER</code></td><td>Spiel → Website</td><td>Raid → Aufstellung → <em>Aufstellung aus Addon abgleichen</em> → <strong>Abgleichen</strong></td><td>Reiter <strong>RAID</strong> → <em>Teilnahme für die Webseite</em> → <strong>EXPORT</strong></td></tr>
            <tr><td><code>RS2;CAL</code></td><td>Spiel → Website</td><td>Raid → Aufstellung → <em>Antworten aus dem Spielkalender</em> → <strong>Abgleichen</strong></td><td>Reiter <strong>RAID</strong> → <em>SPIELKALENDER</em> → <strong>ANTWORTEN FÜR WEBSEITE</strong></td></tr>
            <tr><td><code>RS2;LOOT</code></td><td>Spiel → Website</td><td>Raid → Loot → <em>Loot aus Addon importieren</em> → <strong>Vorschau</strong></td><td>Reiter <strong>VERGABEN</strong> → <strong>EXPORT</strong> (<code>/ledger export</code>)</td></tr>
          </tbody>
        </table>
      </div>
      <h3>Die Einladungsliste</h3>
      <Code>{`RS2;INV;<Raid-ID>;<Raidname>;<Größe>;<JJJJ-MM-TT>;<HH:MM>
<Name>;<TANK|HEAL|DD>;<Klasse>;<Ersatzbank 0/1>[;<SIGNUP|TENTATIVE|DECLINED>]`}</Code>
      <Code>{`RS2;INV;42;ICC25-6;25;2026-11-05;20:00
Thorgrim;TANK;Krieger;0
Mira;HEAL;Paladin;0
Sternfall;DD;Priester;1
Funkel;DD;Magier;0;SIGNUP
Zaudern;HEAL;Druide;0;TENTATIVE
Weitweg;DD;Jäger;0;DECLINED`}</Code>
      <p>Zeilen mit fünftem Feld sind Antworten von der Website ohne Platz in der Aufstellung: angemeldet, unsicher oder abgesagt. Sie gehen nur in den Kalender-Termin. Ältere RaidLedger-Versionen überspringen sie.</p>
      <p>Datum und Startzeit sind <strong>Serverzeit</strong> (die Zeitzone der Raidserie). Ohne Raidserie fehlen die beiden Felder: Einladen geht trotzdem, nur der Kalender-Termin braucht sie. Die Raid-ID merkt sich RaidLedger auch für den Loot-Export.</p>
      <h3>Antworten aus dem Spielkalender</h3>
      <Code>{`RS2;CAL;42;Raidleiter;1760000000
Funkel;accepted
Mira;declined
Thorgrim;confirmed`}</Code>
      <p>Eine Zeile pro Einladung mit der Antwort im Kalender. Übernommen werden nur <code>accepted</code>/<code>signedup</code> (Anmeldung mit Main-Spec) und <code>declined</code>/<code>out</code> („kann nicht“); Status der Raidleitung und fehlende Antworten ändern nichts.</p>
      <h3>Teilnahme und Loot zurück</h3>
      <p>Die Teilnahme listet jeden, der im Raid war, mit Klasse und erster Sichtung, auch Nachzügler und wer früher ging. Der Loot-Export enthält jede Vergabe mit Item, Boss, Empfänger, Wurf, Spec und Wurfbereich. Die Website zeigt erst eine <strong>Vorschau</strong>: Enhanced-Würfe sind als Enhanced vorgewählt, geparkte und gelöschte Zeilen werden angezeigt, aber nie importiert. Ein zweiter Import legt nichts doppelt an.</p>
      <Tip><strong>Reihenfolge am Raidabend:</strong> erst <strong>EXPORT</strong>, dann <strong>NEUER RAID</strong> (beide in VERGABEN). Neuer Raid leert das Buch und die Korrekturen in SPIELER.</Tip>
    </section>

    <section className="card guide-section" id="ledger-befehle" aria-labelledby="ledger-befehle-title">
      <h2 id="ledger-befehle-title">Befehle</h2>
      <div className="table-scroll">
        <table className="table addon-commands">
          <thead><tr><th>Befehl</th><th>Was passiert</th></tr></thead>
          <tbody>
            <tr><td><code>/ledger</code></td><td>Fenster ein- und ausblenden</td></tr>
            <tr><td><code>/ledger setup</code></td><td>Raid aufstellen: Einladungen, Gruppen, Teilnahme</td></tr>
            <tr><td><code>/ledger calendar</code></td><td>Kalender-Termin aus der Einladungsliste, alle eingeladen</td></tr>
            <tr><td><code>/ledger calendar sync</code></td><td>Bestehenden Termin abgleichen: Fehlende einladen, Status nach der Website setzen</td></tr>
            <tr><td><code>/ledger calendar export</code></td><td>Antworten des Termins für die Website</td></tr>
            <tr><td><code>/ledger start</code></td><td>Wurf für das ausgewählte Item starten</td></tr>
            <tr><td><code>/ledger timer &lt;Sek.&gt; [Stechen]</code></td><td>Wurfzeit (10–300 s) und Stechen (5–120 s); <code>/ledger timer reset</code> = 30/15</td></tr>
            <tr><td><code>/ledger pass &lt;Name&gt;</code></td><td>Spieler passt auf das Item</td></tr>
            <tr><td><code>/ledger count &lt;Name&gt; ms|os|none|enh &lt;0-50&gt;</code></td><td>Wurf anders werten</td></tr>
            <tr><td><code>/ledger free</code></td><td>Item als freien BoE-Wurf erneut posten</td></tr>
            <tr><td><code>/ledger roll &lt;Link oder ID&gt;</code></td><td>Item aus den Taschen auswürfeln (wie <kbd>Alt</kbd>+Klick)</td></tr>
            <tr><td><code>/ledger set &lt;#&gt; [Name]</code></td><td>Empfänger von Vergabe # ändern (ohne Name: dein Ziel)</td></tr>
            <tr><td><code>/ledger del &lt;#&gt;</code></td><td>Vergabe # entfernen (im Export als gelöscht)</td></tr>
            <tr><td><code>/ledger spec &lt;#&gt; ms|os|none</code></td><td>Vergabe # für Main-Spec, Second-Spec oder gar nicht zählen</td></tr>
            <tr><td><code>/ledger tally</code></td><td>Items pro Spieler in diesem Raid</td></tr>
            <tr><td><code>/ledger raid &lt;ID&gt;</code></td><td>Raid-ID der Website setzen</td></tr>
            <tr><td><code>/ledger export</code></td><td>Loot-Export anzeigen</td></tr>
            <tr><td><code>/ledger newraid</code></td><td>Buch leeren für den nächsten Raid (vorher exportieren)</td></tr>
            <tr><td><code>/ledger sync</code></td><td>Vergaben dieses Raids aus der Gruppe holen</td></tr>
            <tr><td><code>/ledger allow|deny &lt;Name&gt;</code> · <code>/ledger editors</code></td><td>Bearbeiter hinzufügen oder entfernen; Liste zeigen</td></tr>
            <tr><td><code>/ledger settings</code></td><td>Reiter EINSTELLUNGEN öffnen</td></tr>
            <tr><td><code>/ledger minimap on|off</code></td><td>Minimap-Knopf ein- oder ausblenden</td></tr>
            <tr><td><code>/ledger help</code></td><td>Alle Befehle im Spiel</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section className="card guide-section" id="ledger-installation" aria-labelledby="ledger-installation-title">
      <h2 id="ledger-installation-title">Installation</h2>
      <InstallSteps name="RaidLedger" />
      <Tip><strong>Von RaidSync umsteigen:</strong> RaidLedger kann alles, was RaidSync konnte. Lösch den Ordner <code>Interface\AddOns\RaidSync</code>. Alte <code>RS1</code>-Einladungslisten liest RaidLedger nicht, einfach auf der Website neu kopieren.</Tip>
      <DownloadButton addon={addon} />
    </section>
  </>
}

function RaidWishlistPage({ addon, error }: { addon?: AddonDownload, error: string }) {
  return <>
    <AddonHero
      addon={addon}
      eyebrow="Für jeden Raider"
      audience="Alle Raider"
      pitch="Deine Wunschliste im Spiel: Wenn die Raidleitung ein Item von deiner Liste auswürfelt, würfelst du mit einem Klick den richtigen Bereich, auch mit Enhanced. Oder das Addon würfelt automatisch für dich."
      shot={<Shot name="raidwishlist-fenster" caption="RaidWishlist, Reiter WUNSCHLISTE: das Item ist ERHALTEN" width={466} height={409} />}
    />
    {error && <p className="error" role="alert">{error}</p>}
    <Contents entries={[
      { id: 'wish-funktionen', title: 'Funktionen' },
      { id: 'wish-website', title: 'Zusammenspiel mit der Website' },
      { id: 'wish-befehle', title: 'Befehle' },
      { id: 'wish-installation', title: 'Installation' },
    ]} />

    <section className="card guide-section" id="wish-funktionen" aria-labelledby="wish-funktionen-title">
      <h2 id="wish-funktionen-title">Funktionen</h2>
      <p>Öffnen mit <code>/wish</code> oder dem Minimap-Knopf. Das Fenster öffnet sich auch von selbst, wenn ein Item von deiner Liste ausgewürfelt wird.</p>

      <Feature title="1. Wunschliste ins Spiel holen">
        <ol>
          <li>Auf der Website unter <Link to="/wishlist">Meine Wunschliste</Link> den Charakter wählen, pro Item Main- oder Second-Spec und Enhanced setzen, wichtigstes Item nach oben.</li>
          <li><strong>Export kopieren</strong> klicken.</li>
          <li>Im Spiel <strong>IMPORT</strong> klicken (oder <code>/wish import</code>), mit <kbd>Strg</kbd>+<kbd>V</kbd> einfügen und <strong>IMPORTIEREN</strong>.</li>
        </ol>
        <p>Ein Import ersetzt die alte Liste. Ein Export für einen anderen Charakter wird abgelehnt, damit dein Twink nicht auf die Liste deines Mains würfelt.</p>
        <Tip><strong>EP aktuell halten:</strong> Der Export enthält deinen EP-Stand für Enhanced. Nach neuen EP einfach neu exportieren und importieren.</Tip>
      </Feature>

      <Feature title="2. Würfeln mit einem Klick">
        <p>Startet die Raidleitung einen Wurf auf ein Item von deiner Liste, erscheint oben am Bildschirm ein Hinweis mit Rang und Notiz und zwei Knöpfen: <strong>WÜRFELN MS 21-120</strong> (dein Spec und Bereich) oder <strong>PASSEN</strong>.</p>
        <ul>
          <li><strong>AUTO-WURF: AN</strong> würfelt etwa eine Sekunde nach dem Start für dich, z. B. wenn du kurz weg bist.</li>
          <li>Das Addon würfelt höchstens einmal pro Wurf (nur der erste zählt), beim Stechen nur, wenn du dabei bist, und nicht auf Items, die du in diesem Raid schon bekommen hast.</li>
          <li>Enhanced würfelt <strong>1+X bis 100+X</strong> mit X = EP, höchstens 50. Nach einem Enhanced-Gewinn senkt das Addon seinen EP-Stand bis zum nächsten Export.</li>
        </ul>
      </Feature>

      <Feature title="3. Drei Reiter, jede Liste scrollbar">
        <p>Jeder Reiter ist eine Liste über die ganze Fensterhöhe, die mit dem <strong>Mausrad</strong> scrollt. Unten rechts steht, welche Einträge du siehst („18-30 / 30“), die Reiter zählen ihre Einträge mit.</p>
        <ul>
          <li><strong>WUNSCHLISTE:</strong> deine Wünsche nach Rang, mit Notiz und Würfelbereich. Erhaltene Wünsche zeigen <strong>ERHALTEN</strong>.</li>
          <li><strong>OFFEN:</strong> Beute, die noch ausgewürfelt wird: auf der Leiche, beim Plündermeister oder bei einem Raider geparkt, mit Status <strong>OFFEN</strong>, <strong>WIRD GEWÜRFELT</strong> oder dem Gewinner. Items von deiner Liste sind markiert. Wird ein Wunsch-Item ausgewürfelt, öffnet sich das Fenster hier.</li>
          <li><strong>DIESER RAID:</strong> jede Vergabe der Raidleitung mit Item, Empfänger und Spec. Oben stehen deine Items bisher (Items und Tokens, je Main- und Second-Spec), die bei knappen Würfen entscheiden.</li>
        </ul>
      </Feature>

      <Feature title="4. In deiner Farbe" shot={<Shot name="raidwishlist-farbe" caption="Der Farbknopf oben öffnet die Farbauswahl" width={466} height={409} />}>
        <p>Das Fenster sieht aus wie RaidLedger, in deiner Farbe: Der Farbknopf oben (oder <code>/wish color</code>) öffnet die Farbauswahl, Rechtsklick (oder <code>/wish color reset</code>) geht zurück auf Grün. Die Farbe gilt für alle deine Charaktere. Sehr dunkle Farben hellt das Addon auf, damit die Schrift lesbar bleibt.</p>
      </Feature>

      <Feature title="5. Die Raidleitung weiß, was du willst">
        <p>Deine Liste (Items, Spec, Enhanced, Reihenfolge) geht an RaidLedger der Raidleitung. Sie sieht bei jedem Drop, wie viele und wer ihn wollen. <strong>Notizen bleiben privat.</strong> <code>/wish share off</code> schaltet das Teilen ab.</p>
      </Feature>
    </section>

    <section className="card guide-section" id="wish-website" aria-labelledby="wish-website-title">
      <h2 id="wish-website-title">Zusammenspiel mit der Website</h2>
      <p>Die Website exportiert die Liste eines Charakters als Text mit <code>;</code> getrennt. Der Kopf enthält Charakter, Exportzeit und deinen EP-Stand.</p>
      <Code>{`WL2;<Charakter>;<Exportzeit>;<EP-Stand>
<Item-ID>;<ms|os>;<Enhanced 0/1>;<Rang>;<Notiz>`}</Code>
      <Code>{`WL2;Sternfall;1792130000;70
50415;ms;1;1;BiS-Waffe
52027;ms;0;2;
50074;os;0;3;für den Heilspec`}</Code>
      <ul>
        <li><code>ms</code> würfelt <code>/rnd 100</code>, <code>os</code> würfelt <code>/rnd 99</code>.</li>
        <li>Enhanced 1 mit 70 EP würfelt 51–150, mit 20 EP 21–120. Second-Spec kann nicht Enhanced sein.</li>
        <li>Was du gewinnst, trägt die Raidleitung über den Loot-Import von RaidLedger auf der Website ein. Die EP für Enhanced zieht die Website dabei ab.</li>
      </ul>
    </section>

    <section className="card guide-section" id="wish-befehle" aria-labelledby="wish-befehle-title">
      <h2 id="wish-befehle-title">Befehle</h2>
      <div className="table-scroll">
        <table className="table addon-commands">
          <thead><tr><th>Befehl</th><th>Was passiert</th></tr></thead>
          <tbody>
            <tr><td><code>/wish</code></td><td>Fenster ein- und ausblenden</td></tr>
            <tr><td><code>/wish import</code></td><td>Wunschliste einfügen</td></tr>
            <tr><td><code>/wish auto on|off</code></td><td>Automatisch würfeln oder nachfragen</td></tr>
            <tr><td><code>/wish sync</code></td><td>Vergaben dieses Raids von der Raidleitung holen</td></tr>
            <tr><td><code>/wish share on|off</code></td><td>Wunschliste mit der Raidleitung teilen (Standard: an)</td></tr>
            <tr><td><code>/wish minimap on|off</code></td><td>Minimap-Knopf ein- oder ausblenden</td></tr>
            <tr><td><code>/wish color</code> · <code>/wish color reset</code></td><td>Fensterfarbe wählen oder zurück auf Grün</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section className="card guide-section" id="wish-installation" aria-labelledby="wish-installation-title">
      <h2 id="wish-installation-title">Installation</h2>
      <InstallSteps name="RaidWishlist" />
      <DownloadButton addon={addon} />
    </section>
  </>
}

export function AddonsPage({ addonID }: { addonID?: string }) {
  const { addons, error } = useAddons()
  if (addonID === 'raidledger' || addonID === 'raidwishlist') {
    const addon = addons.find((entry) => entry.name === addonNames[addonID])
    return <div className="guide-page addon-page">
      {addonID === 'raidledger' ? <RaidLedgerPage addon={addon} error={error} /> : <RaidWishlistPage addon={addon} error={error} />}
    </div>
  }
  if (addonID) return <p className="muted">Dieses Addon gibt es hier nicht. <Link to="/addons">Alle Addons</Link></p>
  return <div className="guide-page addon-page"><Overview addons={addons} error={error} /></div>
}
