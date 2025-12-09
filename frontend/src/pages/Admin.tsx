import { useQuery } from '@tanstack/react-query';
import { motion } from 'framer-motion';
import { Users, Cpu, Shield, Mail, Clock } from 'lucide-react';
import { format } from 'date-fns';
import { adminApi, dashboardApi } from '../services/api';

export default function Admin() {
  const { data: stats } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: dashboardApi.getStats,
  });

  const { data: users } = useQuery({
    queryKey: ['admin-users'],
    queryFn: adminApi.getAllUsers,
  });

  const { data: devices } = useQuery({
    queryKey: ['admin-devices'],
    queryFn: adminApi.getAllDevices,
  });

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-8"
      >
        <div className="flex items-center gap-3 mb-2">
          <Shield className="w-8 h-8 text-primary-400" />
          <h1 className="text-3xl font-bold">
            <span className="gradient-text">Admin Panel</span>
          </h1>
        </div>
        <p className="text-dark-400">
          System administration and monitoring
        </p>
      </motion.div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
        {[
          { label: 'Total Users', value: stats?.total_users ?? 0, icon: Users, color: 'from-purple-500 to-pink-500' },
          { label: 'Total Devices', value: stats?.total_devices ?? 0, icon: Cpu, color: 'from-primary-500 to-blue-500' },
          { label: 'Online Devices', value: stats?.online_devices ?? 0, icon: Cpu, color: 'from-green-500 to-emerald-500' },
          { label: 'Avg Temp', value: `${(stats?.avg_temperature ?? 0).toFixed(1)}°C`, icon: Users, color: 'from-orange-500 to-red-500' },
        ].map((stat, i) => (
          <motion.div
            key={stat.label}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.1 }}
            className="glass rounded-xl p-5"
          >
            <div className={`w-10 h-10 rounded-lg bg-gradient-to-br ${stat.color} flex items-center justify-center mb-3`}>
              <stat.icon className="w-5 h-5 text-white" />
            </div>
            <p className="text-2xl font-bold">{stat.value}</p>
            <p className="text-sm text-dark-400">{stat.label}</p>
          </motion.div>
        ))}
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        {/* Users Table */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Users className="w-5 h-5 text-primary-400" />
            Users ({users?.length ?? 0})
          </h2>
          <div className="space-y-3 max-h-96 overflow-auto">
            {users?.map((user) => (
              <div key={user.id} className="flex items-center gap-3 p-3 rounded-xl bg-dark-800/50">
                {user.picture ? (
                  <img src={user.picture} alt={user.name} className="w-10 h-10 rounded-full" />
                ) : (
                  <div className="w-10 h-10 rounded-full bg-dark-700 flex items-center justify-center">
                    <Users className="w-5 h-5 text-dark-400" />
                  </div>
                )}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="font-medium truncate">{user.name}</p>
                    {user.is_admin && (
                      <span className="px-2 py-0.5 rounded text-xs bg-purple-500/20 text-purple-400">
                        Admin
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-dark-400 truncate flex items-center gap-1">
                    <Mail className="w-3 h-3" />
                    {user.email}
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-xs text-dark-500 flex items-center gap-1">
                    <Clock className="w-3 h-3" />
                    {format(new Date(user.created_at), 'MMM d, yyyy')}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </motion.div>

        {/* All Devices Table */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Cpu className="w-5 h-5 text-accent-400" />
            All Devices ({devices?.length ?? 0})
          </h2>
          <div className="space-y-3 max-h-96 overflow-auto">
            {devices?.map((device) => (
              <div key={device.id} className="flex items-center gap-3 p-3 rounded-xl bg-dark-800/50">
                <div className={`w-10 h-10 rounded-lg ${
                  device.is_online ? 'bg-green-500/20' : 'bg-dark-700'
                } flex items-center justify-center`}>
                  <Cpu className={`w-5 h-5 ${device.is_online ? 'text-green-400' : 'text-dark-400'}`} />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium truncate">{device.name}</p>
                  <p className="text-sm text-dark-400 truncate">
                    {device.platform} • {device.mac || 'Unknown MAC'}
                  </p>
                </div>
                <div className="text-right">
                  <span className={`px-2 py-1 rounded-full text-xs ${
                    device.is_online 
                      ? 'bg-green-500/20 text-green-400' 
                      : 'bg-dark-600 text-dark-400'
                  }`}>
                    {device.is_online ? 'Online' : 'Offline'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </motion.div>
      </div>
    </div>
  );
}

