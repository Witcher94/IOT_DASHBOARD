import { format, formatDistanceToNow } from 'date-fns';
import { uk, enUS } from 'date-fns/locale';
import { useSettingsStore } from '../contexts/settingsStore';

export const getLocale = () => {
  const language = useSettingsStore.getState().language;
  return language === 'uk' ? uk : enUS;
};

export const formatDateTime = (date: string | Date, pattern = 'dd.MM.yyyy HH:mm') => {
  return format(new Date(date), pattern, { locale: getLocale() });
};

export const formatTime = (date: string | Date) => {
  return format(new Date(date), 'HH:mm:ss', { locale: getLocale() });
};

export const formatRelative = (date: string | Date) => {
  return formatDistanceToNow(new Date(date), { 
    addSuffix: true, 
    locale: getLocale() 
  });
};

export const formatChartTime = (date: string | Date) => {
  return format(new Date(date), 'HH:mm', { locale: getLocale() });
};

// Hook version for components
export const useDateFormat = () => {
  const { language } = useSettingsStore();
  const locale = language === 'uk' ? uk : enUS;
  
  return {
    formatDateTime: (date: string | Date, pattern = 'dd.MM.yyyy HH:mm') => 
      format(new Date(date), pattern, { locale }),
    formatTime: (date: string | Date) => 
      format(new Date(date), 'HH:mm:ss', { locale }),
    formatRelative: (date: string | Date) => 
      formatDistanceToNow(new Date(date), { addSuffix: true, locale }),
    formatChartTime: (date: string | Date) => 
      format(new Date(date), 'HH:mm', { locale }),
  };
};


