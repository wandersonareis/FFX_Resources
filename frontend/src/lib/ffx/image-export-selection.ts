'use client';

import { useEffect } from 'react';
import { createStore } from '@tanstack/store';
import { useSelector } from '@tanstack/react-store';

export type ImageExportFormat = 'dds' | 'png' | 'dds+png';

export const IMAGE_EXPORT_FORMAT_LABELS: Record<ImageExportFormat, string> = {
  dds: 'DDS',
  png: 'PNG',
  'dds+png': 'DDS + PNG',
};

const FORMAT_STORAGE_KEY = 'ffx.image-export-format';

function loadStoredFormat(): ImageExportFormat {
  try {
    const stored = localStorage.getItem(FORMAT_STORAGE_KEY);
    if (stored === 'png' || stored === 'dds+png') {
      return stored;
    }
    return 'dds';
  } catch {
    return 'dds';
  }
}

interface ImageExportFormatState {
  revision: number;
  format: ImageExportFormat;
}

const imageExportFormatStore = createStore<ImageExportFormatState>({
  revision: 0,
  format: 'dds',
});

class ImageExportFormatStore {
  private format: ImageExportFormat = 'dds';

  getSnapshot(): ImageExportFormatState {
    return imageExportFormatStore.state;
  }

  private notify(): void {
    imageExportFormatStore.setState((prev) => ({
      revision: prev.revision + 1,
      format: this.format,
    }));
  }

  setFormat(format: ImageExportFormat): void {
    if (this.format === format) return;
    this.format = format;
    try {
      localStorage.setItem(FORMAT_STORAGE_KEY, format);
    } catch {
      // localStorage indisponível — segue só em memória
    }
    this.notify();
  }

  formatOf(): ImageExportFormat {
    return this.format;
  }
}

export const imageExportFormatSelection = new ImageExportFormatStore();

export function useImageExportFormat(): ImageExportFormatState {
  const snapshot = useSelector(imageExportFormatStore);

  useEffect(() => {
    imageExportFormatSelection.setFormat(loadStoredFormat());
  }, []);

  return snapshot;
}