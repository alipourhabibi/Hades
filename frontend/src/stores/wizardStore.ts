'use client';

import { create } from 'zustand';

interface WizardState {
  open: boolean;
  openWizard: () => void;
  closeWizard: () => void;
}

export const useWizardStore = create<WizardState>((set) => ({
  open: false,
  openWizard: () => set({ open: true }),
  closeWizard: () => set({ open: false }),
}));
