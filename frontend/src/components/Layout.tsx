import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import {
  LayoutDashboard,
  Cpu,
  Settings,
  LogOut,
  User,
  Shield,
  Wifi,
  Moon,
  Sun,
} from 'lucide-react';
import { useAuthStore } from '../contexts/authStore';
import { useSettingsStore, useTranslation } from '../contexts/settingsStore';

export default function Layout() {
  const { user, logout } = useAuthStore();
  const { theme, toggleTheme } = useSettingsStore();
  const t = useTranslation();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const navItems = [
    { to: '/', icon: LayoutDashboard, label: t.dashboard },
    { to: '/devices', icon: Cpu, label: t.devices },
    { to: '/settings', icon: Settings, label: t.settings },
  ];

  if (user?.is_admin) {
    navItems.push({ to: '/admin', icon: Shield, label: t.admin });
  }

  return (
    <div className="min-h-screen flex">
      {/* Sidebar */}
      <motion.aside
        initial={{ x: -100, opacity: 0 }}
        animate={{ x: 0, opacity: 1 }}
        className="w-72 bg-dark-900/80 backdrop-blur-xl border-r border-dark-700/50 flex flex-col"
      >
        {/* Logo */}
        <div className="p-6 border-b border-dark-700/50">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-primary-500 to-accent-400 flex items-center justify-center">
              <Wifi className="w-5 h-5 text-dark-950" />
            </div>
            <div>
              <h1 className="text-lg font-bold gradient-text">IoT Dashboard</h1>
              <p className="text-xs text-dark-400">Mesh Network Monitor</p>
            </div>
          </div>
        </div>

        {/* Navigation */}
        <nav className="flex-1 p-4 space-y-2">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-200 ${
                  isActive
                    ? 'bg-gradient-to-r from-primary-500/20 to-accent-400/10 text-white border border-primary-500/30'
                    : 'text-dark-300 hover:text-white hover:bg-dark-800/50'
                }`
              }
            >
              <item.icon className="w-5 h-5" />
              <span className="font-medium">{item.label}</span>
            </NavLink>
          ))}
        </nav>

        {/* User section */}
        <div className="p-4 border-t border-dark-700/50">
          <div className="flex items-center gap-3 px-4 py-3 rounded-xl bg-dark-800/50">
            {user?.picture ? (
              <img
                src={user.picture}
                alt={user.name}
                className="w-10 h-10 rounded-full border-2 border-primary-500/30"
              />
            ) : (
              <div className="w-10 h-10 rounded-full bg-dark-700 flex items-center justify-center">
                <User className="w-5 h-5 text-dark-400" />
              </div>
            )}
            <div className="flex-1 min-w-0">
              <p className="font-medium truncate">{user?.name}</p>
              <p className="text-xs text-dark-400 truncate">{user?.email}</p>
            </div>
          </div>
          
          <div className="mt-3 flex gap-2">
            <button
              onClick={toggleTheme}
              className="flex items-center justify-center gap-2 px-3 py-2 text-sm text-dark-300 hover:text-white rounded-lg hover:bg-dark-800/50 transition-colors"
              title={theme === 'dark' ? 'Switch to light' : 'Switch to dark'}
            >
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
            </button>
            <button
              onClick={handleLogout}
              className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm text-red-400 hover:text-red-300 rounded-lg hover:bg-red-500/10 transition-colors"
            >
              <LogOut className="w-4 h-4" />
              {t.logout}
            </button>
          </div>
        </div>
      </motion.aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  );
}

