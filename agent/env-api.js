import fs from 'fs/promises';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV_PATH = path.join(__dirname, '..', '.env');

export async function readEnv() {
  try {
    const content = await fs.readFile(ENV_PATH, 'utf-8');
    const result = {};
    content.split('\n').forEach(line => {
      const match = line.match(/^([^=]+)=(.*)$/);
      if (match) {
        result[match[1].trim()] = match[2].trim();
      }
    });
    return result;
  } catch (err) {
    return {};
  }
}

export async function writeEnv(updates) {
  const current = await readEnv();
  const merged = { ...current, ...updates };
  const content = Object.entries(merged)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n');
  await fs.writeFile(ENV_PATH, content, 'utf-8');
  return merged;
}
