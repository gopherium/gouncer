Feature: Inviting a person to activate an account
  An administrator adds a person by email, name and role. The account waits
  unconfirmed and the person gets a link to choose a password. The link
  goes out by mail, or comes back to the administrator when the app sends
  no mail. The answer never tells whether the address already held an
  account. A resend ends the link sent before. Opening a live link sets the
  password, confirms the account and signs the person in. A client that
  keeps trying dead links is slowed down.

  Scenario: An invitation mails the activation link
    Given the app sends mail
    And an administrator is signed in
    When "maria@example.com" is invited as "Maria Perez" with the role "member"
    Then the answer says the link was delivered
    And "maria@example.com" received 1 mail greeting "Maria Perez"
    And the account of "maria@example.com" waits for activation with the role "member"

  Scenario: Without mail the link comes back to the administrator
    Given the app sends no mail
    And an administrator is signed in
    When "maria@example.com" is invited as "Maria Perez" with the role "member"
    Then the answer carries the activation link
    And the account of "maria@example.com" waits for activation with the role "member"

  Scenario: A role the reader may not give is refused
    Given the app sends mail
    And a moderator is signed in
    When "maria@example.com" is invited as "Maria Perez" with the role "admin"
    Then the invitation is refused because the role may not be given
    And no account holds "maria@example.com"
    And no mail was sent

  Scenario: An invitation never tells whether the address already held an account
    Given the app sends mail
    And an administrator is signed in
    And "maria@example.com" already holds an active account
    When "maria@example.com" is invited as "Someone Else" with the role "member"
    Then the answer says the link was delivered
    And no mail was sent
    And the account of "maria@example.com" is unchanged

  Scenario: Inviting an address still waiting ends the link sent before
    Given the app sends no mail
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    When "maria@example.com" is invited as "Someone Else" with the role "member"
    Then the earlier link no longer activates
    And the latest link activates
    And the account of "maria@example.com" keeps the name "Maria Perez"

  Scenario: A failed delivery is reported and the invitation stays
    Given the mail relay refuses every mail
    And an administrator is signed in
    When "maria@example.com" is invited as "Maria Perez" with the role "member"
    Then the invitation is reported undelivered
    And the account of "maria@example.com" waits for activation with the role "member"

  Scenario: A resend ends the link sent before
    Given the app sends no mail
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    When the invitation to "maria@example.com" is sent again
    Then the earlier link no longer activates
    And the latest link activates

  Scenario Outline: A resend to an address with no invitation to renew tells nothing
    Given the app sends mail
    And an administrator is signed in
    And "maria@example.com" already holds an active account
    When the invitation to "<address>" is sent again
    Then the answer says the link was delivered
    And no mail was sent

    Examples:
      | address             |
      | maria@example.com   |
      | nobody@example.com  |

  Scenario: Opening the link sets the password and signs the person in
    Given the app sends no mail
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    When the person opens the latest link and chooses the password "a password Maria chose"
    Then the person is signed in as "maria@example.com"
    And "maria@example.com" can log in with the password "a password Maria chose"

  Scenario: A password too short is refused and the link still works
    Given the app sends no mail
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    When the person opens the latest link and chooses the password "short"
    Then the password is refused as too short
    And the latest link activates

  Scenario: A link works once
    Given the app sends no mail
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    And the person opened the latest link and chose the password "a password Maria chose"
    When the person opens the latest link and chooses the password "another password entirely"
    Then the link is refused as no longer valid
    And "maria@example.com" can log in with the password "a password Maria chose"

  Scenario: A client that keeps trying dead links is slowed down
    Given the app sends no mail
    And each client may try 2 dead links
    And an administrator is signed in
    And "maria@example.com" was invited as "Maria Perez" with the role "member"
    And a client tried 2 dead links
    When the same client opens the latest link and chooses the password "a password Maria chose"
    Then the activation is refused as too many attempts
    And the account of "maria@example.com" waits for activation with the role "member"
