// Encounter artwork uses Blizzard achievement icons, served by Wowhead.
// Multi-NPC encounters count once; optional bosses are included.
export interface BossEncounter { name: string; icon: string }

const encounter = (name: string, icon: string): BossEncounter => ({ name, icon })

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
    encounter('Flammenleviathan', 'flameleviathan_01'),
    encounter('Ignis', 'ignis_01'),
    encounter('Klingenschuppe', 'razorscale'),
    encounter('XT-002 Dekonstruktor', 'xt002deconstructor_01'),
    encounter('Versammlung des Eisens', 'theironcouncil_01'),
    encounter('Kologarn', 'kologarn_01'),
    encounter('Auriaya', 'auriaya_01'),
    encounter('Hodir', 'hodir_01'),
    encounter('Thorim', 'thorim'),
    encounter('Freya', 'freya_01'),
    encounter('Mimiron', 'mimiron_01'),
    encounter('General Vezax', 'generalvezax_01'),
    encounter('Yogg-Saron', 'yoggsaron_01'),
    encounter('Algalon der Beobachter', 'algalon_01'),
  ],
  '624': [
    encounter('Archavon', 'archavon'),
    encounter('Emalon', 'emalon_01'),
    encounter('Koralon', 'koralon'),
    encounter('Toravon', 'toravon'),
  ],
  '631': [
    encounter('Lord Mark’gar', 'lordmarrowgar'),
    encounter('Lady Todeswisper', 'ladydeathwhisper'),
    encounter('Luftschiffkampf', 'gunshipbattle'),
    encounter('Todesbringer Saurfang', 'saurfang'),
    encounter('Fauldarm', 'festergut'),
    encounter('Modermiene', 'rotface'),
    encounter('Professor Seuchenmord', 'profputricide'),
    encounter('Rat der Blutprinzen', 'bloodprincecouncil'),
    encounter('Blutkönigin Lana’thel', 'bloodqueenlanathel'),
    encounter('Valithria Traumwandler', 'valithriadreamwalker'),
    encounter('Sindragosa', 'sindragosa'),
    encounter('Der Lichkönig', 'lichking'),
  ],
  '649': [
    encounter('Bestien von Nordend', 'gormoktheimpaler'),
    encounter('Lord Jaraxxus', 'lordjaraxxus'),
    encounter('Fraktionschampions', 'factionchampions'),
    encounter('Zwillingsval’kyr', 'twinvalkyr'),
    encounter('Anub’arak', 'anubarak'),
  ],
  '724': [
    encounter('Baltharus der Kriegsjünger', 'baltharus'),
    encounter('Saviana Flammenschlund', 'saviana'),
    encounter('General Zarithrian', 'zarithrian'),
    encounter('Halion', 'halion'),
  ],
}

export function getBossesForInstance(instanceId?: string | null): BossEncounter[] {
  return instanceId ? INSTANCE_BOSSES[instanceId] ?? [] : []
}
