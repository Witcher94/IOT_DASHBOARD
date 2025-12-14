import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Users, Cpu, Shield, Mail, Clock, Trash2, ShieldCheck, ShieldOff, ChevronDown, ChevronUp, Eye, ExternalLink } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import { adminApi, dashboardApi } from '../services/api';
import type { User, Device } from '../types';

export default function Admin() {
  const queryClient = useQueryClient();
  const [expandedUser, setExpandedUser] = useState<string | null>(null);
  const [userDevices, setUserDevices] = useState<Record<string, Device[]>>({});

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

  const deleteUserMutation = useMutation({
    mutationFn: adminApi.deleteUser,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
      toast.success('User deleted successfully');
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to delete user');
    },
  });

  const updateRoleMutation = useMutation({
    mutationFn: ({ id, isAdmin }: { id: string; isAdmin: boolean }) => 
      adminApi.updateUserRole(id, isAdmin),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      toast.success('Role updated successfully');
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to update role');
    },
  });

  const handleDeleteUser = (user: User) => {
    if (confirm(`Are you sure you want to delete user "${user.name}"?\nThis will also delete all their devices and data.`)) {
      deleteUserMutation.mutate(user.id);
    }
  };

  const handleToggleAdmin = (user: User) => {
    const action = user.is_admin ? 'remove admin rights from' : 'grant admin rights to';
    if (confirm(`Are you sure you want to ${action} "${user.name}"?`)) {
      updateRoleMutation.mutate({ id: user.id, isAdmin: !user.is_admin });
    }
  };

  const handleExpandUser = async (userId: string) => {
    if (expandedUser === userId) {
      setExpandedUser(null);
      return;
    }
    
    setExpandedUser(userId);
    if (!userDevices[userId]) {
      try {
        const devices = await adminApi.getUserDevices(userId);
        setUserDevices(prev => ({ ...prev, [userId]: devices }));
      } catch (error) {
        console.error('Failed to load user devices:', error);
      }
    }
  };

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
          { label: 'Avg Temp', value: `${Math.round(stats?.avg_temperature ?? 0)}°C`, icon: Users, color: 'from-orange-500 to-red-500' },
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
          <div className="space-y-3 max-h-[500px] overflow-auto">
            {users?.map((user) => (
              <div key={user.id} className="rounded-xl bg-dark-800/50 overflow-hidden">
                <div className="flex items-center gap-3 p-3">
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
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleToggleAdmin(user)}
                      className={`p-2 rounded-lg transition-colors ${
                        user.is_admin 
                          ? 'bg-purple-500/20 text-purple-400 hover:bg-purple-500/30' 
                          : 'bg-dark-700 text-dark-400 hover:bg-dark-600'
                      }`}
                      title={user.is_admin ? 'Remove admin' : 'Make admin'}
                    >
                      {user.is_admin ? <ShieldOff className="w-4 h-4" /> : <ShieldCheck className="w-4 h-4" />}
                    </button>
                    <button
                      onClick={() => handleDeleteUser(user)}
                      className="p-2 rounded-lg bg-red-500/20 text-red-400 hover:bg-red-500/30 transition-colors"
                      title="Delete user"
                      disabled={user.is_admin}
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => handleExpandUser(user.id)}
                      className="p-2 rounded-lg bg-dark-700 text-dark-400 hover:bg-dark-600 transition-colors"
                      title="View devices"
                    >
                      {expandedUser === user.id ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                    </button>
                  </div>
                </div>
                
                {/* Expanded user devices */}
                {expandedUser === user.id && (
                  <div className="px-3 pb-3 border-t border-dark-700">
                    <p className="text-xs text-dark-500 py-2">
                      <Clock className="w-3 h-3 inline mr-1" />
                      Joined {format(new Date(user.created_at), 'MMM d, yyyy')}
                    </p>
                    <p className="text-sm text-dark-400 mb-2">Devices:</p>
                    {userDevices[user.id]?.length ? (
                      <div className="space-y-2">
                        {userDevices[user.id].map(device => (
                          <Link 
                            key={device.id} 
                            to={`/devices/${device.id}`}
                            className="flex items-center gap-2 text-sm p-2 bg-dark-900/50 rounded-lg hover:bg-dark-700 transition-colors group"
                          >
                            <div className={`w-2 h-2 rounded-full ${device.is_online ? 'bg-green-400' : 'bg-dark-500'}`} />
                            <span className="flex-1">{device.name}</span>
                            <span className="text-dark-500">({device.device_type === 'gateway' ? 'Raspberry Pi' : device.platform || 'ESP'})</span>
                            <ExternalLink className="w-3 h-3 text-dark-500 opacity-0 group-hover:opacity-100 transition-opacity" />
                          </Link>
                        ))}
                      </div>
                    ) : (
                      <p className="text-sm text-dark-500 italic">No devices</p>
                    )}
                  </div>
                )}
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
          <div className="space-y-3 max-h-[500px] overflow-auto">
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
                    {device.platform || 'Unknown'} • {device.mac || 'Unknown MAC'}
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
                  {device.last_seen && (
                    <p className="text-xs text-dark-500 mt-1">
                      {format(new Date(device.last_seen), 'HH:mm')}
                    </p>
                  )}
                </div>
              </div>
            ))}
            {!devices?.length && (
              <p className="text-center text-dark-500 py-8">No devices registered</p>
            )}
          </div>
        </motion.div>
      </div>
    </div>
  );
}
