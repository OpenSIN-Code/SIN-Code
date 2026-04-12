import * as fs from 'node:fs';
import * as path from 'node:path';
import * as tar from 'tar';
import { uploadFile, downloadFile } from '@huggingface/hub';
import cron from 'node-cron';

const AUTH_DIR = path.join(process.cwd(), '.wwebjs_auth');
const BACKUP_FILE = path.join(process.cwd(), 'auth_backup.tar.gz');

const hfToken = process.env.HF_TOKEN;
const datasetRepo = process.env.HF_DATASET_REPO;

export async function backupAuthSession(): Promise<void> {
  if (!hfToken || !datasetRepo) {
    return;
  }

  if (!fs.existsSync(AUTH_DIR)) {
    return;
  }

  try {
    await tar.c({ gzip: true, file: BACKUP_FILE, cwd: process.cwd() }, ['.wwebjs_auth']);

    const fileBuffer = fs.readFileSync(BACKUP_FILE);
    const blob = new Blob([fileBuffer], { type: 'application/gzip' });

    await uploadFile({
      repo: { type: 'dataset', name: datasetRepo },
      credentials: { accessToken: hfToken },
      file: {
        path: 'auth_backup.tar.gz',
        content: blob
      },
      commitTitle: 'Auto-Backup: Session - ' + new Date().toISOString()
    });
  } catch (error) {
    console.error('[HF-Sync] Error during backup:', error);
  }
}

export async function restoreAuthSession(): Promise<void> {
  if (!hfToken || !datasetRepo) {
    return;
  }

  try {
    const response = await downloadFile({
      repo: { type: 'dataset', name: datasetRepo },
      credentials: { accessToken: hfToken },
      path: 'auth_backup.tar.gz'
    });

    if (!response) {
      return;
    }

    const buffer = await response.arrayBuffer();
    fs.writeFileSync(BACKUP_FILE, Buffer.from(buffer));
    
    await tar.x({ file: BACKUP_FILE, cwd: process.cwd() });
    console.log('[HF-Sync] Restored session from dataset.');
  } catch (error: any) {
    if (!error.message?.includes('404')) {
      console.error('[HF-Sync] Restore error:', error);
    }
  }
}

export function startKeepAlivePing(): void {
  const spaceUrl = process.env.SPACE_HOST ? 'https://' + process.env.SPACE_HOST : 'http://localhost:' + (process.env.PORT || 7860);
  
  cron.schedule('*/5 * * * *', async () => {
    try {
      await fetch(spaceUrl);
    } catch (e) {
      // ignore
    }
  });
}

export function startAutoBackup(): void {
  cron.schedule('*/10 * * * *', async () => {
    await backupAuthSession();
  });
}