import { useEffect, useState } from 'react'
import { api } from '../api'
import { Link } from '../router'

// The privacy page (FR-30.1, FR-30.4, FR-30.8). It is readable without a session;
// the responsible person comes from configuration through a public endpoint (SC-033).

interface PrivacyContact {
  name: string
  address: string
}

export function PrivacyPage() {
  const [contact, setContact] = useState<PrivacyContact | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    api.get<PrivacyContact>('/api/v1/privacy-contact')
      .then((value) => { if (active) setContact(value) })
      .catch(() => { if (active) setError('Die Kontaktdaten konnten nicht geladen werden. Bitte die Seite neu laden.') })
    return () => { active = false }
  }, [])

  return <div className="guide-page privacy-page">
    <div className="page-heading">
      <div>
        <p className="eyebrow">Datenschutz</p>
        <h1>Datenschutzerklärung</h1>
      </div>
    </div>
    <p className="guide-lead">Diese Seite ist die Raidverwaltung der Gilde „Bratwurst des Schicksals“ auf dem Realm Rising-Gods. Hier steht, welche Daten gespeichert werden, warum, wie lange und wie du Auskunft oder Löschung bekommst.</p>

    <section className="card guide-section" aria-labelledby="verantwortlich-title">
      <h2 id="verantwortlich-title">Verantwortlich</h2>
      {error && <p className="error" role="alert">{error}</p>}
      {!error && !contact && <p className="muted">Lade&hellip;</p>}
      {contact && (contact.name || contact.address
        ? <p>Verantwortlich im Sinne der DSGVO ist <strong>{contact.name}</strong>. Kontakt: <strong>{contact.address}</strong></p>
        : <p className="muted">Die Kontaktdaten sind auf diesem Server nicht eingetragen.</p>)}
    </section>

    <section className="card guide-section" aria-labelledby="daten-title">
      <h2 id="daten-title">Welche Daten gespeichert werden</h2>
      <ul>
        <li><strong>Konto:</strong> Spielername, Anmeldename, Rolle und ein Passwort-Hash (nie das Passwort selbst). Wenn du sie einrichtest: Daten für Authenticator-App, Wiederherstellungscodes und Sicherheitsschlüssel.</li>
        <li><strong>Charaktere:</strong> Name, Klasse, Skillungen und Gearscore, geladen aus dem öffentlichen Rising-Gods-Profil. Die rohe Antwort des Profils wird nicht gespeichert.</li>
        <li><strong>Raidbetrieb:</strong> Anmeldungen, Absagen, Aufstellungen, Beute, Enhance Points, Itemsperren, Wunschlisten und Umfragestimmen.</li>
        <li><strong>Änderungsprotokoll:</strong> Wer wann welche Daten geändert hat (siehe unten).</li>
        <li><strong>Technisch nötig:</strong> Ein Sitzungs-Cookie nach der Anmeldung und ein Schutz-Cookie gegen gefälschte Anfragen. Es gibt keine Werbung, keine Analyse und keine Einbindung fremder Server. Anmeldeversuche und abgelaufene Sitzungen werden gelöscht, sobald sie nicht mehr gebraucht werden.</li>
      </ul>
      <p>Ohne Anmeldung sichtbar sind nur der Raidkalender, die Spielerliste mit Gildenrang, die Statistiken (Zählungen, keine Items und keine Enhance Points) und die Hilfeseiten.</p>
    </section>

    <section className="card guide-section" aria-labelledby="grundlage-title">
      <h2 id="grundlage-title">Zweck und Rechtsgrundlage</h2>
      <p>Die Daten dienen der Planung und Abrechnung der Gildenraids. Eine nachvollziehbare Beute- und Punktehistorie, die jedes Mitglied prüfen und anfechten kann, ist ein berechtigtes Interesse der Gilde (Art. 6 Abs. 1 Buchst. f DSGVO). Für optionale Funktionen wie die Anmeldung mit zweitem Faktor gilt dasselbe Interesse an einem sicheren Konto.</p>
    </section>

    <section className="card guide-section" aria-labelledby="protokoll-title">
      <h2 id="protokoll-title">Änderungsprotokoll</h2>
      <p>Jede Änderung an Raids, Beute, Punkten und Konten wird im Änderungsprotokoll festgehalten. Die Einträge bleiben ohne zeitliche Begrenzung bestehen, weil die Beute- und Punktehistorie sonst nicht mehr überprüfbar wäre. Nur Administratoren können das Protokoll lesen. Wenn du die Löschung deiner Daten verlangst, bleiben die Einträge erhalten, dein Name wird darin aber ausgeblendet.</p>
    </section>

    <section className="card guide-section" aria-labelledby="pseudonym-title">
      <h2 id="pseudonym-title">Löschung ist Pseudonymisierung</h2>
      <p>Auf Anfrage macht ein Administrator dein Konto unkenntlich. Dabei werden Anmeldename, Passwort, Sitzungen, Daten für den zweiten Faktor und gespeicherte Rising-Gods-Profildaten gelöscht. Dein Spielername wird durch den Namen deines Hauptcharakters ersetzt (ohne Charakter: „Ehemaliges Mitglied #Nummer“).</p>
      <p><strong>Das macht deine Daten nicht anonym.</strong> Aufstellungen, Beute, Enhance-Point-Buchungen und Charakternamen in der Raidhistorie bleiben erhalten. Charakternamen sind im Spiel öffentlich, deshalb können Menschen, die deine Charaktere kennen, dich weiterhin wiedererkennen. Das ist eine Pseudonymisierung, keine Anonymisierung.</p>
    </section>

    <section className="card guide-section" aria-labelledby="rechte-title">
      <h2 id="rechte-title">Deine Rechte</h2>
      <p>Du kannst Auskunft über deine gespeicherten Daten, Berichtigung und Löschung verlangen sowie der Verarbeitung widersprechen. Schreibe dazu an den oben genannten Kontakt. Eine Auskunft bekommst du als maschinenlesbare Datei, die ein Administrator für dich erstellt. Eine Selbstbedienung zum Löschen gibt es nicht, die Löschung führt ein Administrator aus. Du kannst dich außerdem bei der zuständigen Datenschutz-Aufsichtsbehörde beschweren.</p>
      <p>Tipps für ein sicheres Konto findest du unter <Link to="/kontosicherheit">Kontosicherheit</Link>.</p>
    </section>
  </div>
}
