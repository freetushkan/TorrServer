// Fork-only settings (not in upstream). The single include point in the
// upstream settings component; add new fork options here, not there.
// Translations are registered here, so upstream locale files stay untouched.
import { useTranslation } from 'react-i18next'
import { FormControlLabel, FormGroup, FormHelperText, Switch } from '@material-ui/core'

import i18n from '../../i18n'
import PerUserSettings from './PerUserSettings'
import { SettingSectionLabel } from './style'

const translations = {
  en: {
    Viewed: 'Viewing history',
    TrackTimecode: 'Save playback position',
    TrackTimecodeHint: 'Players that report the position get it back and resume where they stopped.',
  },
  ru: {
    Viewed: 'История просмотра',
    TrackTimecode: 'Сохранять позицию воспроизведения',
    TrackTimecodeHint: 'Плееры, которые передают позицию, получают её обратно и продолжают с места остановки.',
  },
}
translations.ua = translations.ru

Object.entries(translations).forEach(([lng, keys]) => {
  i18n.addResourceBundle(lng, 'translation', { SettingsDialog: { Fork: keys } }, true, false)
})

export default function ForkSettings({ settings, inputForm }) {
  const { t } = useTranslation()
  const { TrackTimecode } = settings || {}

  return (
    <>
      <SettingSectionLabel style={{ marginTop: '20px' }}>{t('SettingsDialog.Fork.Viewed')}</SettingSectionLabel>
      <FormGroup>
        <FormControlLabel
          control={
            <Switch checked={Boolean(TrackTimecode)} onChange={inputForm} id='TrackTimecode' color='secondary' />
          }
          label={t('SettingsDialog.Fork.TrackTimecode')}
          labelPlacement='start'
        />
        <FormHelperText margin='none'>{t('SettingsDialog.Fork.TrackTimecodeHint')}</FormHelperText>
      </FormGroup>
      <PerUserSettings settings={settings} inputForm={inputForm} />
    </>
  )
}
