import { motion } from 'framer-motion';
import { Settings as SettingsIcon, Globe, Moon, Sun, Monitor } from 'lucide-react';
import { useSettingsStore, useTranslation, type Language, type Theme } from '../contexts/settingsStore';

export default function Settings() {
  const { language, theme, setLanguage, setTheme } = useSettingsStore();
  const t = useTranslation();

  const languages: { value: Language; label: string; flag: string }[] = [
    { value: 'uk', label: 'Українська', flag: '🇺🇦' },
    { value: 'en', label: 'English', flag: '🇬🇧' },
  ];

  const themes: { value: Theme; label: string; icon: typeof Moon }[] = [
    { value: 'dark', label: t.darkTheme, icon: Moon },
    { value: 'light', label: t.lightTheme, icon: Sun },
  ];

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-8"
      >
        <div className="flex items-center gap-3 mb-2">
          <SettingsIcon className="w-8 h-8 text-primary-400" />
          <h1 className="text-3xl font-bold">
            <span className="gradient-text">{t.settingsTitle}</span>
          </h1>
        </div>
        <p className="text-dark-400">{t.appearance}</p>
      </motion.div>

      <div className="grid gap-6 max-w-2xl">
        {/* Language */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="glass rounded-2xl p-6"
        >
          <div className="flex items-center gap-3 mb-4">
            <Globe className="w-5 h-5 text-primary-400" />
            <h2 className="text-lg font-semibold">{t.languageLabel}</h2>
          </div>
          
          <div className="grid grid-cols-2 gap-3">
            {languages.map((lang) => (
              <button
                key={lang.value}
                onClick={() => setLanguage(lang.value)}
                className={`flex items-center gap-3 p-4 rounded-xl transition-all ${
                  language === lang.value
                    ? 'bg-primary-500/20 border-2 border-primary-500/50 text-white'
                    : 'bg-dark-800/50 border-2 border-transparent hover:border-dark-600 text-dark-300'
                }`}
              >
                <span className="text-2xl">{lang.flag}</span>
                <span className="font-medium">{lang.label}</span>
              </button>
            ))}
          </div>
        </motion.div>

        {/* Theme */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="glass rounded-2xl p-6"
        >
          <div className="flex items-center gap-3 mb-4">
            <Monitor className="w-5 h-5 text-accent-400" />
            <h2 className="text-lg font-semibold">{t.themeLabel}</h2>
          </div>
          
          <div className="grid grid-cols-2 gap-3">
            {themes.map((t) => (
              <button
                key={t.value}
                onClick={() => setTheme(t.value)}
                className={`flex items-center justify-center gap-3 p-4 rounded-xl transition-all ${
                  theme === t.value
                    ? 'bg-primary-500/20 border-2 border-primary-500/50 text-white'
                    : 'bg-dark-800/50 border-2 border-transparent hover:border-dark-600 text-dark-300'
                }`}
              >
                <t.icon className="w-5 h-5" />
                <span className="font-medium">{t.label}</span>
              </button>
            ))}
          </div>
        </motion.div>

        {/* Preview */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4">Preview</h2>
          <div className={`p-4 rounded-xl ${theme === 'dark' ? 'bg-dark-950' : 'bg-gray-100'}`}>
            <div className={`text-sm ${theme === 'dark' ? 'text-white' : 'text-gray-900'}`}>
              {language === 'uk' ? (
                <p>🇺🇦 Ласкаво просимо до IoT Dashboard!</p>
              ) : (
                <p>🇬🇧 Welcome to IoT Dashboard!</p>
              )}
            </div>
          </div>
        </motion.div>
      </div>
    </div>
  );
}

