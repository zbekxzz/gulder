/** Русские подписи справочников (§15: русский интерфейс). */
export const occasionTypes: Record<string, string> = {
  birthday: "День рождения",
  anniversary: "Годовщина",
  march8: "8 Марта",
  valentine: "День влюблённых",
  wedding: "Свадьба",
  apology: "Извинение",
  gratitude: "Благодарность",
  congrats: "Поздравление",
  sympathy: "Соболезнование",
  just_because: "Без повода",
};
export const relationships: Record<string, string> = {
  partner: "Партнёр",
  spouse: "Супруг(а)",
  mother: "Мама",
  father: "Папа",
  relative: "Родственник",
  friend: "Друг",
  colleague: "Коллега",
  boss: "Руководитель",
  teacher: "Учитель",
  other: "Другое",
};
export const places: Record<string, string> = {
  home: "Дома",
  office: "В офисе",
  restaurant: "В ресторане",
  event: "На мероприятии",
  hospital: "В больнице",
};
export const emotions: Record<string, string> = {
  joy: "Радость",
  gratitude: "Благодарность",
  romance: "Романтика",
  support: "Поддержка",
  respect: "Уважение",
  apology: "Извинение",
};
export const styles: Record<string, string> = {
  classic: "Классика",
  minimal: "Минимализм",
  rustic: "Рустик",
  bright: "Ярко",
};
export const formats: Record<string, string> = {
  bouquet: "Букет",
  box: "Коробка",
  basket: "Корзина",
  single: "Один цветок",
};
export const levels: Record<string, string> = {
  none: "Нет",
  low: "Слабая",
  medium: "Средняя",
  strong: "Сильная",
};
export const scentLevels: Record<string, string> = {
  none: "без запаха",
  low: "слабый запах",
  medium: "заметный запах",
  strong: "сильный запах",
};
export const colors: Record<string, string> = {
  white: "Белый",
  pink: "Розовый",
  red: "Красный",
  yellow: "Жёлтый",
  peach: "Персиковый",
  orange: "Оранжевый",
  purple: "Сиреневый",
  blue: "Голубой",
  green: "Зелёный",
  cream: "Кремовый",
};
export const genera: Record<string, string> = {
  rosa: "Розы",
  lilium: "Лилии",
  tulipa: "Тюльпаны",
  chrysanthemum: "Хризантемы",
  helianthus: "Подсолнухи",
  gerbera: "Герберы",
  dianthus: "Гвоздики",
  phalaenopsis: "Орхидеи",
  hydrangea: "Гортензии",
  paeonia: "Пионы",
  narcissus: "Нарциссы",
  alstroemeria: "Альстромерии",
  gypsophila: "Гипсофила",
  freesia: "Фрезии",
  eucalyptus: "Эвкалипт",
  hyacinthus: "Гиацинты",
};
export const allergens: Record<string, string> = {
  pollen: "Пыльца",
  "genus:lilium": "Лилии",
  "genus:rosa": "Розы",
  "family:asteraceae": "Астровые (герберы, хризантемы, подсолнухи)",
  "family:liliaceae": "Лилейные (лилии, тюльпаны)",
  "family:amaryllidaceae": "Амариллисовые (нарциссы)",
  "food:nuts": "Орехи",
  "food:peanut": "Арахис",
  "food:milk": "Молоко",
  "food:gluten": "Глютен",
  "food:egg": "Яйца",
  "food:soy": "Соя",
  "food:honey": "Мёд",
  "food:cocoa": "Какао и шоколад",
  fragrance: "Парфюмерные отдушки",
  latex: "Латекс",
};
export const pets: Record<string, string> = {
  cat: "Кошка",
  dog: "Собака",
  horse: "Лошадь",
  rabbit: "Кролик",
  bird: "Птица",
  rodent: "Грызун",
};
export const petsShort: Record<string, string> = { cat: "кошек", dog: "собак", horse: "лошадей" };
export const giftCategories: Record<string, string> = {
  card: "Открытка",
  sweets: "Сладости",
  soft_toy: "Мягкая игрушка",
  fragrance: "Ароматы",
  jewelry: "Украшения",
  candle: "Свеча",
  other: "Другое",
};
export const missingLabels: Record<string, string> = {
  allergies: "аллергии",
  pets: "домашние животные",
  asthma: "астма",
  scent_sensitivity: "отношение к запахам",
};

export const label = (map: Record<string, string>, key: string | null | undefined) =>
  key ? (map[key] ?? key) : "—";
