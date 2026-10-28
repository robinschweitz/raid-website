// Encounter artwork uses Blizzard achievement icons, served by Wowhead.
// Multi-NPC encounters count once; optional bosses are included.
// id is the catalog boss ID, which recorded kills refer to; only catalog instances have one.
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
    encounter('Lord Mark’gar', 'lordmarrowgar', 'ICCLordMarrowgar'),
    encounter('Lady Todeswisper', 'ladydeathwhisper', 'ICCLadyDeathwhisper'),
    encounter('Luftschiffkampf', 'gunshipbattle', 'ICCGunshipBattle'),
    encounter('Todesbringer Saurfang', 'saurfang', 'ICCSaurfang'),
    encounter('Fauldarm', 'festergut', 'ICCFestergut'),
    encounter('Modermiene', 'rotface', 'ICCRotface'),
    encounter('Professor Seuchenmord', 'profputricide', 'ICCPutricide'),
    encounter('Rat der Blutprinzen', 'bloodprincecouncil', 'ICCCouncil'),
    encounter('Blutkönigin Lana’thel', 'bloodqueenlanathel', 'ICCLanathel'),
    encounter('Valithria Traumwandler', 'valithriadreamwalker', 'ICCValithria'),
    encounter('Sindragosa', 'sindragosa', 'ICCSindragosa'),
    encounter('Der Lichkönig', 'lichking', 'ICCLichKing'),
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
