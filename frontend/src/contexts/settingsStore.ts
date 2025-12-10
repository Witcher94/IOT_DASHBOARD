import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Language = 'uk' | 'en';
export type Theme = 'dark' | 'light';

interface SettingsState {
  language: Language;
  theme: Theme;
  setLanguage: (lang: Language) => void;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set, get) => ({
      language: 'uk',
      theme: 'dark',
      
      setLanguage: (language: Language) => set({ language }),
      
      setTheme: (theme: Theme) => {
        set({ theme });
        document.documentElement.classList.toggle('light', theme === 'light');
      },
      
      toggleTheme: () => {
        const newTheme = get().theme === 'dark' ? 'light' : 'dark';
        set({ theme: newTheme });
        document.documentElement.classList.toggle('light', newTheme === 'light');
      },
    }),
    {
      name: 'settings-storage',
      onRehydrateStorage: () => (state) => {
        // Apply theme on load
        if (state?.theme === 'light') {
          document.documentElement.classList.add('light');
        }
      },
    }
  )
);

// Translations
export const translations = {
  uk: {
    // Navigation
    dashboard: 'Дашборд',
    devices: 'Пристрої',
    admin: 'Адмін',
    settings: 'Налаштування',
    logout: 'Вийти',
    
    // Dashboard
    totalDevices: 'Всього пристроїв',
    online: 'Онлайн',
    avgTemperature: 'Сер. температура',
    avgHumidity: 'Сер. вологість',
    users: 'Користувачі',
    meshNetworkStatus: 'Статус Mesh мережі',
    realtimeConnections: 'Підключення в реальному часі',
    live: 'Наживо',
    noDevicesYet: 'Пристроїв ще немає',
    addFirstDevice: 'Додайте перший ESP пристрій',
    
    // Devices
    manageDevices: 'Управління пристроями ESP32/ESP8266',
    addDevice: 'Додати пристрій',
    searchDevices: 'Пошук пристроїв...',
    noDevicesFound: 'Пристроїв не знайдено',
    deviceCreated: 'Пристрій створено!',
    copyToken: 'Скопіюй токен',
    tokenWarning: 'Скопіюй токен зараз. З міркувань безпеки він більше не буде показаний.',
    done: 'Готово',
    
    // Device Detail
    temperature: 'Температура',
    humidity: 'Вологість',
    wifiSignal: 'WiFi сигнал',
    freeMemory: "Вільна пам'ять",
    sensorData: 'Дані датчиків',
    wifiNetworks: 'WiFi мережі',
    quickCommands: 'Швидкі команди',
    reboot: 'Перезавантаження',
    toggleDHT: 'Перемкнути DHT',
    toggleMesh: 'Перемкнути Mesh',
    recentCommands: 'Останні команди',
    deviceToken: 'Токен пристрою',
    regenerate: 'Перегенерувати',
    tokenRegenerated: 'Токен перегенеровано!',
    
    // Settings
    settingsTitle: 'Налаштування',
    appearance: 'Зовнішній вигляд',
    languageLabel: 'Мова',
    themeLabel: 'Тема',
    darkTheme: 'Темна',
    lightTheme: 'Світла',
    ukrainian: 'Українська',
    english: 'English',
    
    // Common
    loading: 'Завантаження...',
    error: 'Помилка',
    save: 'Зберегти',
    cancel: 'Скасувати',
    delete: 'Видалити',
    confirm: 'Підтвердити',
    neverConnected: 'Ще не підключався',
  },
  en: {
    // Navigation
    dashboard: 'Dashboard',
    devices: 'Devices',
    admin: 'Admin',
    settings: 'Settings',
    logout: 'Logout',
    
    // Dashboard
    totalDevices: 'Total Devices',
    online: 'Online',
    avgTemperature: 'Avg Temperature',
    avgHumidity: 'Avg Humidity',
    users: 'Users',
    meshNetworkStatus: 'Mesh Network Status',
    realtimeConnections: 'Real-time device connections',
    live: 'Live',
    noDevicesYet: 'No devices yet',
    addFirstDevice: 'Add your first ESP device to get started',
    
    // Devices
    manageDevices: 'Manage your ESP32/ESP8266 devices',
    addDevice: 'Add Device',
    searchDevices: 'Search devices...',
    noDevicesFound: 'No devices found',
    deviceCreated: 'Device Created!',
    copyToken: 'Copy token',
    tokenWarning: 'Copy the token now. For security reasons, it won\'t be shown again.',
    done: 'Done',
    
    // Device Detail
    temperature: 'Temperature',
    humidity: 'Humidity',
    wifiSignal: 'WiFi Signal',
    freeMemory: 'Free Memory',
    sensorData: 'Sensor Data',
    wifiNetworks: 'WiFi Networks',
    quickCommands: 'Quick Commands',
    reboot: 'Reboot',
    toggleDHT: 'Toggle DHT',
    toggleMesh: 'Toggle Mesh',
    recentCommands: 'Recent Commands',
    deviceToken: 'Device Token',
    regenerate: 'Regenerate',
    tokenRegenerated: 'Token regenerated!',
    
    // Settings
    settingsTitle: 'Settings',
    appearance: 'Appearance',
    languageLabel: 'Language',
    themeLabel: 'Theme',
    darkTheme: 'Dark',
    lightTheme: 'Light',
    ukrainian: 'Українська',
    english: 'English',
    
    // Common
    loading: 'Loading...',
    error: 'Error',
    save: 'Save',
    cancel: 'Cancel',
    delete: 'Delete',
    confirm: 'Confirm',
    neverConnected: 'Never connected',
  },
};

export const useTranslation = () => {
  const { language } = useSettingsStore();
  return translations[language];
};

