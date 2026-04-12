import { createClient, SupabaseClient } from '@supabase/supabase-js';

// Database coupling to global SIN-Supabase A2A (Oracle OCI VM 200GB)
// SSOT: https://docs.google.com/document/d/1RtoHn4I0GntuEEOHHkqoh_dMuGzgMwQz7_8oxAOpQbw/edit?tab=t.zglvro7czbod

let supabaseInstance: SupabaseClient | null = null;

export function getSupabaseClient(): SupabaseClient {
  if (supabaseInstance) {
    return supabaseInstance;
  }

  const supabaseUrl = process.env.SIN_SUPABASE_URL;
  const supabaseKey = process.env.SIN_SUPABASE_SERVICE_ROLE_KEY;

  if (!supabaseUrl || !supabaseKey) {
    console.warn('[SIN-Supabase] Warning: SIN_SUPABASE_URL or SIN_SUPABASE_SERVICE_ROLE_KEY is missing. DB coupling is inactive.');
    return createClient(supabaseUrl || 'https://dummy.supabase.co', supabaseKey || 'dummy');
  }

  supabaseInstance = createClient(supabaseUrl, supabaseKey);
  return supabaseInstance;
}