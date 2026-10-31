# @gopherium/react-auth

Session authentication for React apps backed by gouncer's authkit.

## Entry points

- `@gopherium/react-auth` is the headless core: the auth API client,
  `useSession`, `useLogout`, `sessionQueryKey`, the slot-based
  `AuthGate`, `createAuthQueryClient`, `isSessionRevoked`, and
  `catalogFor`, which answers the `es-ES` and `fr-FR` catalogues of the
  text domain `DOMAIN`.
- `@gopherium/react-auth/admin` is the user administration API client.
- `@gopherium/react-auth/wpds` renders the WordPress Design System UI:
  `LoginScreen` (brand prop), `AccountPanel`, `usersMenuItem`,
  `SetupCodeStep`, which asks for the one-time setup code as Jenkins'
  "Unlock Jenkins" does, and `SetupAccountStep`, the account rows of
  WordPress's installer with its password strength meter, and
  `PasswordField` with `usePasswordField`, that step's password row for
  any form that sets a password. Import
  `@gopherium/react-auth/wpds/style.css` alongside it.
- `@gopherium/react-auth/wpds/users` is the Users page on WordPress
  DataViews: `UsersPage`, `usersListDefaults`, the view it opens on as
  data for godmin's `useListView`, `RoleSelect`, the role menu with the
  fewest capabilities first, `roleLabelOf`, and `errorMessageOf`, the
  words the list toasts a failed account write in. It is its own entry,
  so an app that only signs people in never resolves its peers.
- `@gopherium/react-auth/testing` is the msw harness: `server`,
  `installTestEnvironment`, `seedSession`, `defaultUser`, and canned
  handlers for the auth endpoints.

## Mount

```tsx
const client = createAuthQueryClient()

<QueryClientProvider client={client}>
  <AuthGate loginScreen={(onLogin) => <LoginScreen brand="MyApp" onLogin={onLogin} />}>
    <App />
  </AuthGate>
</QueryClientProvider>
```

## Setup wizard rows

`SetupAccountStep` shows the wizard's own rows in `before`, above
Name, and in `after`, below Your Email. They line up with the step's
rows only when they use the same hooks:

- `className="gopherium-setup__row"` on each row.
- `render={<div />}` on `Field.Root`, so the row adds no gap of its
  own.
- `className="gopherium-setup__label"` on the row's label.
- `className="gopherium-setup__checkbox"` on a `CheckboxControl`.
- `className="gopherium-setup__help"` on a help line under the field.

A checkbox row is a plain `<div className="gopherium-setup__row">`
with a `Field.VisualLabel`, as the step's Confirm Password row is.

```tsx
<SetupAccountStep
  code={code}
  submitLabel="Install MyApp"
  onCreated={onCreated}
  before={
    <Field.Root className="gopherium-setup__row" render={<div />}>
      <Field.Label className="gopherium-setup__label">Site Title</Field.Label>
      <div>
        <Input value={title} onChange={(event) => setTitle(event.target.value)} />
      </div>
    </Field.Root>
  }
/>
```

## Password field

An Add User form takes the step's password row with WordPress's
"Generate password" button first. `labelPosition="top"` stacks each
label above its field as a godmin form does, where the installer's
default `side` puts it beside the field. Send `password.value` once
`password.ready` is true.

```tsx
const password = usePasswordField({ length: 24, userInputs: [name, email] })

<PasswordField password={password} generateFirst labelPosition="top" readOnly={sending} />
```

`required` and `customValidity` take the shapes `@wordpress/ui`'s
`ValidatedInputControl` takes. `required` adds "(Required)" to the
label. A message shows under the field and its meter, in red with the
warning icon, once the field is left, an `invalid` event fires or the
form's `reportValidity()` runs. While it is `invalid` the field has
`aria-invalid` and the browser holds the form back. When the app sets
a message after a send, it calls the form's `reportValidity()` in an
effect that runs after that render, so the message shows at once.

An Add User form leaves `required` out, as WordPress's Add User shows
no "(required)" by the password it always generates. It catches an
empty password with `customValidity`, as it does for Email and Name.

```tsx
<PasswordField
  password={password}
  generateFirst
  labelPosition="top"
  customValidity={message === undefined ? undefined : { type: 'invalid', message }}
/>
```

## Users page

The app holds the view in its address with godmin's `useListView`, as
every other list does, and hands the page the result and its own words
as data. Its route keeps the view with `validateSearch: listSearch`.

```tsx
const list = useListView<View>(usersListDefaults({ perPage: settings?.pageSize }))

<UsersPage
  list={list}
  pageSizes={settings?.pageSizes}
  sizesFailed={settingsFailed}
  roleLabels={{ admin: __('Administrator'), member: __('Member') }}
  addLink={<Link to="/users/new" />}
  headingLevel={2}
/>
```

- An app with no page size setting passes a fixed `perPage` to
  `usersListDefaults`. The list shows as loading until it has a page
  size or `sizesFailed`.
- A row the server marks `editable: false`, and the reader's own row,
  offer no action.
- `errorMessages` gives words to the error codes only the app answers,
  from its server or its own transport. The codes authkit answers carry
  WordPress's words.
- `columns` adds a column after Role as data: a label and, for each
  account id, the text, the words a screen reader hears and an optional
  link element. Pass its id to `usersListDefaults` as well, to show it.
- `showAvatars={false}` drops the Avatar field when the site shows none.

## Consumer requirements

- `react` and `@tanstack/react-query` are peer dependencies; the app
  must own exactly one copy of each. With workspace or linked installs,
  add both to your bundler's dedupe list.
- The client calls same-origin `/api/auth/*` and `/api/users` paths. In
  development, proxy `/api` to your backend so the `__Host-` session
  cookie is first-party.
- `/wpds` needs the `@wordpress/ui` and `@wordpress/icons` peers.
- `/wpds/users` also needs `@gopherium/godmin` 0.28 or 0.29,
  `@gopherium/gottext`, `@wordpress/dataviews`, `@wordpress/components`
  and `@wordpress/date`, and godmin's `Toaster` above the page, whose
  toasts show each name whole. The app loads the DataViews and
  components stylesheets, and `@gopherium/react-auth/wpds/style.css`,
  which clips a long email under the name and sizes `RoleSelect` on a
  phone.
- The account step loads zxcvbn, about 400 kB gzipped, as its own chunk
  the first time it renders. Vite reports that chunk as larger than
  500 kB.

## License

Apache-2.0. Copyright © 2026 Manuel 'SirLouen' Camargo.
