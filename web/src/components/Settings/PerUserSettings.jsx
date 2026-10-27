// Per-user data separation (fork feature, not in upstream).
// Self-contained: translations are registered here, so upstream locale files
// and the settings component only need a single include line.
import i18n from '../../i18n'
import { useTranslation } from 'react-i18next'
import { FormControlLabel, FormGroup, FormHelperText, Switch } from '@material-ui/core'

import { SettingSectionLabel } from './style'

const translations = {
  en: {
    Users: 'Users',
    CurrentUser: 'Current user',
    PerUserData: 'Per-user data separation',
    PerUserDataHint: 'When enabled, torrents and viewed history are isolated by authenticated account.',
    PerUserDataForcedHint: "Forced on by the '--peruserdata' flag.",
  },
  ru: {
    Users: 'Пользователи',
    CurrentUser: 'Текущий пользователь',
    PerUserData: 'Разделение данных по пользователям',
    PerUserDataHint: 'При включении торренты и история просмотров разделяются по авторизованным учётным записям.',
    PerUserDataForcedHint: "Принудительно включено флагом запуска '--peruserdata'.",
  },
}
translations.ua = translations.ru

Object.entries(translations).forEach(([lng, keys]) =>
  i18n.addResourceBundle(lng, 'translation', { SettingsDialog: { PerUser: keys } }, true, false),
)

export default function PerUserSettings({ settings, inputForm }) {
  const { t } = useTranslation()
  const { PerUserData, PerUserDataForced, CurrentUser } = settings || {}

  return (
    <>
      <SettingSectionLabel style={{ marginTop: '20px' }}>{t('SettingsDialog.PerUser.Users')}</SettingSectionLabel>
      <FormGroup>
        <FormControlLabel
          control={
            <Switch
              checked={Boolean(PerUserData || PerUserDataForced)}
              onChange={inputForm}
              id='PerUserData'
              color='secondary'
              disabled={Boolean(PerUserDataForced)}
            />
          }
          label={t('SettingsDialog.PerUser.PerUserData')}
          labelPlacement='start'
        />
        <FormHelperText margin='none'>
          {PerUserDataForced && (
            <>
              {t('SettingsDialog.PerUser.PerUserDataForcedHint')}
              <br />
            </>
          )}
          {t('SettingsDialog.PerUser.PerUserDataHint')}
          {CurrentUser && (
            <>
              <br />
              {t('SettingsDialog.PerUser.CurrentUser')}: {CurrentUser}
            </>
          )}
        </FormHelperText>
      </FormGroup>
    </>
  )
}
