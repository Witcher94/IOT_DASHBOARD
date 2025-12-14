import { useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Wifi } from 'lucide-react';
import { useAuthStore } from '../contexts/authStore';
import { authApi } from '../services/api';

export default function AuthCallback() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { setToken, setUser } = useAuthStore();

  useEffect(() => {
    const token = searchParams.get('token');
    
    if (token) {
      setToken(token);
      
      // Fetch user info
      authApi.getCurrentUser()
        .then((user) => {
          setUser(user);
          navigate('/', { replace: true });
        })
        .catch((error) => {
          console.error('Failed to fetch user:', error);
          navigate('/login', { replace: true });
        });
    } else {
      navigate('/login', { replace: true });
    }
  }, [searchParams, setToken, setUser, navigate]);

  return (
    <div className="min-h-screen flex items-center justify-center">
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        className="text-center"
      >
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ duration: 2, repeat: Infinity, ease: "linear" }}
          className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-primary-500 to-accent-400 mb-6"
        >
          <Wifi className="w-8 h-8 text-dark-950" />
        </motion.div>
        <h2 className="text-xl font-semibold text-white mb-2">Authenticating...</h2>
        <p className="text-dark-400">Please wait while we sign you in</p>
      </motion.div>
    </div>
  );
}



