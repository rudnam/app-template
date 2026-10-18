import { t, useLanguage } from '../i18n';
import { languages, pickedLanguage, setLanguage, type Language } from '../language';

/** Picks the app's language, or follows the device's (the empty choice). */
export function LanguagePicker() {
  useLanguage();
  return (
    <select
      className="language"
      aria-label={t('Language')}
      value={pickedLanguage() ?? ''}
      onChange={(e) => setLanguage((e.target.value || null) as Language | null)}
    >
      <option value="">{t('Auto')}</option>
      {languages.map((l) => (
        <option key={l.code} value={l.code} lang={l.code}>
          {l.name}
        </option>
      ))}
    </select>
  );
}
