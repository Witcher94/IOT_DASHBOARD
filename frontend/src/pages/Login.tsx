import { motion } from 'framer-motion';
import { Wifi, Cpu, Activity, Globe } from 'lucide-react';
import { authApi } from '../services/api';

export default function Login() {
  const handleGoogleLogin = () => {
    window.location.href = authApi.getGoogleLoginUrl();
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-6 relative overflow-hidden">
      {/* Background effects */}
      <div className="absolute inset-0 overflow-hidden">
        <div className="absolute top-1/4 -left-20 w-96 h-96 bg-primary-500/20 rounded-full blur-3xl animate-pulse-slow" />
        <div className="absolute bottom-1/4 -right-20 w-96 h-96 bg-accent-400/20 rounded-full blur-3xl animate-pulse-slow" style={{ animationDelay: '1s' }} />
        
        {/* Grid pattern */}
        <div className="absolute inset-0 bg-[linear-gradient(rgba(78,195,255,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(78,195,255,0.03)_1px,transparent_1px)] bg-[size:60px_60px]" />
      </div>

      {/* Floating icons */}
      <motion.div
        animate={{ y: [0, -20, 0] }}
        transition={{ duration: 6, repeat: Infinity, ease: "easeInOut" }}
        className="absolute top-20 left-20 text-primary-500/30"
      >
        <Cpu className="w-16 h-16" />
      </motion.div>
      <motion.div
        animate={{ y: [0, 20, 0] }}
        transition={{ duration: 5, repeat: Infinity, ease: "easeInOut" }}
        className="absolute bottom-32 right-32 text-accent-400/30"
      >
        <Activity className="w-20 h-20" />
      </motion.div>
      <motion.div
        animate={{ y: [0, -15, 0] }}
        transition={{ duration: 7, repeat: Infinity, ease: "easeInOut" }}
        className="absolute top-40 right-40 text-primary-500/20"
      >
        <Globe className="w-12 h-12" />
      </motion.div>

      {/* Login card */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.6 }}
        className="relative z-10 w-full max-w-md"
      >
        <div className="glass rounded-3xl p-8 glow-box">
          {/* Logo */}
          <div className="text-center mb-8">
            <motion.div
              initial={{ scale: 0 }}
              animate={{ scale: 1 }}
              transition={{ delay: 0.2, type: "spring", stiffness: 200 }}
              className="inline-flex items-center justify-center w-20 h-20 rounded-2xl bg-gradient-to-br from-primary-500 to-accent-400 mb-6 shadow-lg shadow-primary-500/30"
            >
              <Wifi className="w-10 h-10 text-dark-950" />
            </motion.div>
            
            <h1 className="text-3xl font-bold mb-2">
              <span className="gradient-text">IoT Dashboard</span>
            </h1>
            <p className="text-dark-400">
              Monitor your ESP32/ESP8266 mesh network
            </p>
          </div>

          {/* Features */}
          <div className="grid grid-cols-2 gap-4 mb-8">
            {[
              { icon: Cpu, label: 'Device Monitoring' },
              { icon: Activity, label: 'Real-time Data' },
              { icon: Wifi, label: 'Mesh Network' },
              { icon: Globe, label: 'OTA Updates' },
            ].map((feature, index) => (
              <motion.div
                key={feature.label}
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.3 + index * 0.1 }}
                className="flex items-center gap-2 text-sm text-dark-300"
              >
                <feature.icon className="w-4 h-4 text-primary-400" />
                {feature.label}
              </motion.div>
            ))}
          </div>

          {/* Google Login Button */}
          <motion.button
            whileHover={{ scale: 1.02 }}
            whileTap={{ scale: 0.98 }}
            onClick={handleGoogleLogin}
            className="w-full flex items-center justify-center gap-3 px-6 py-4 bg-white text-gray-800 rounded-2xl font-semibold transition-all hover:shadow-lg hover:shadow-white/20"
          >
            <svg className="w-5 h-5" viewBox="0 0 24 24">
              <path
                fill="currentColor"
                d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
              />
              <path
                fill="#34A853"
                d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
              />
              <path
                fill="#FBBC05"
                d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
              />
              <path
                fill="#EA4335"
                d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
              />
            </svg>
            Sign in with Google
          </motion.button>

          <p className="text-center text-xs text-dark-500 mt-6">
            By signing in, you agree to our Terms of Service and Privacy Policy
          </p>
        </div>
      </motion.div>
    </div>
  );
}

