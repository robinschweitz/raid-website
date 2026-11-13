// Encounter artwork uses Blizzard achievement icons, served by Wowhead.
// Multi-NPC encounters count once; optional bosses are included.
// id is the stored catalog boss ID (loot_catalog_bosses.id), which recorded kills refer to; only catalog instances have one.
export interface BossEncounter { name: string; icon: string; id?: string }

const encounter = (name: string, icon: string, id?: string): BossEncounter => ({ name, icon, id })

export const INSTANCE_BOSSES: Record<string, BossEncounter[]> = {
  '533': [
    encounter("Anub'Rekhan", 'anubrekan'),
    encounter('Großwitwe Faerlina', 'grandwidowfaerlina'),
    encounter('Maexxna', 'maexxna'),
    encounter('Noth der Seuchenfürst', 'noththeplaguebringer'),
    encounter('Heigan der Unreine', 'heigantheunclean'),
    encounter('Loatheb', 'loatheb'),
    encounter('Instrukteur Razuvious', 'instructorrazuvious'),
    encounter('Gothik der Ernter', 'gothiktheharvester'),
    encounter('Die Vier Reiter', 'thefourhorsemen'),
    encounter('Flickwerk', 'patchwerk'),
    encounter('Grobbulus', 'grobbulus'),
    encounter('Gluth', 'gluth'),
    encounter('Thaddius', 'thaddius'),
    encounter('Saphiron', 'sapphiron'),
    encounter("Kel'Thuzad", 'kelthuzad_01'),
  ],
  '603': [
    encounter('Flammenleviathan', 'flameleviathan_01', '33113'),
    encounter('Ignis', 'ignis_01', '33118'),
    encounter('Klingenschuppe', 'razorscale', '33186'),
    encounter('XT-002 Dekonstruktor', 'xt002deconstructor_01', '33293'),
    encounter('Versammlung des Eisens', 'theironcouncil_01', 'encounter:603:iron-council'),
    encounter('Kologarn', 'kologarn_01', '32930'),
    encounter('Auriaya', 'auriaya_01', '33515'),
    encounter('Hodir', 'hodir_01', '32845'),
    encounter('Thorim', 'thorim', '32865'),
    encounter('Freya', 'freya_01', '32906'),
    encounter('Mimiron', 'mimiron_01', '33350'),
    encounter('General Vezax', 'generalvezax_01', '33271'),
    encounter('Yogg-Saron', 'yoggsaron_01', '33288'),
    encounter('Algalon der Beobachter', 'algalon_01', '32871'),
  ],
  '624': [
    encounter('Archavon', 'archavon'),
    encounter('Emalon', 'emalon_01'),
    encounter('Koralon', 'koralon'),
    encounter('Toravon', 'toravon'),
  ],
  '631': [
    encounter('Lord Mark’gar', 'lordmarrowgar', '36612'),
    encounter('Lady Todeswisper', 'ladydeathwhisper', '36855'),
    encounter('Luftschiffkampf', 'gunshipbattle', 'encounter:631:gunship'),
    encounter('Todesbringer Saurfang', 'saurfang', '37813'),
    encounter('Fauldarm', 'festergut', '36626'),
    encounter('Modermiene', 'rotface', '36627'),
    encounter('Professor Seuchenmord', 'profputricide', '36678'),
    encounter('Rat der Blutprinzen', 'bloodprincecouncil', 'encounter:631:blood-prince-council'),
    encounter('Blutkönigin Lana’thel', 'bloodqueenlanathel', '37955'),
    encounter('Valithria Traumwandler', 'valithriadreamwalker', '36789'),
    encounter('Sindragosa', 'sindragosa', '36853'),
    encounter('Der Lichkönig', 'lichking', '36597'),
  ],
  '649': [
    encounter('Bestien von Nordend', 'gormoktheimpaler', 'encounter:649:northrend-beasts'),
    encounter('Lord Jaraxxus', 'lordjaraxxus', '34780'),
    encounter('Fraktionschampions', 'factionchampions', 'encounter:649:faction-champions'),
    encounter('Zwillingsval’kyr', 'twinvalkyr', 'encounter:649:twin-valkyr'),
    encounter('Anub’arak', 'anubarak', '34564'),
  ],
  '724': [
    encounter('Baltharus der Kriegsjünger', 'baltharus'),
    encounter('Saviana Flammenschlund', 'saviana'),
    encounter('General Zarithrian', 'zarithrian'),
    encounter('Halion', 'halion', '39863'),
  ],
}

export function getBossesForInstance(instanceId?: string | null): BossEncounter[] {
  return instanceId ? INSTANCE_BOSSES[instanceId] ?? [] : []
}
