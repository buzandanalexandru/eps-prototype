# Spec Delta

## Purpose

Lets a fan ask a storefront for a plan in plain words and get a suggested cart from that shop's published products, which the fan confirms; the concierge never holds, buys or pays.

## ADDED Requirements

### Requirement: Request to a suggested cart
The system SHALL turn a fan's request in Romanian or English into a short plan and a suggested cart made only of products published in that shop, with quantities and all-in prices taken from the published sale data.

#### Scenario: Weekend for four
- **WHEN** a fan asks "weekend for 4, hip-hop, 300 RON each" on a shop
- **THEN** the panel shows a plan and a cart of existing products for 4 people with all-in prices and a total, or says that the budget cannot be met and shows the closest option
- Test: apps/web/tests/unit/ai-concierge.test.ts

#### Scenario: Product not in the shop
- **WHEN** the model suggests a product that is not in the shop's published products, or a quantity above the per-account cap
- **THEN** that item is dropped or capped and the fan is told why
- Test: apps/web/tests/unit/ai-concierge.test.ts

### Requirement: The fan confirms
The system SHALL add the suggestion to the cart only when the fan presses Add to cart, and SHALL never create a hold, an order or a payment on its own.

#### Scenario: Fan confirms the cart
- **WHEN** the fan presses Add to cart on a suggestion
- **THEN** the items go into the normal cart and checkout follows the usual hold, queue and payment rules
- Test: apps/web/tests/e2e/ai-concierge.spec.ts

### Requirement: Labelled and private
The system SHALL label concierge answers as AI suggestions and SHALL not store the conversation beyond the fan's browser session.

#### Scenario: Panel label
- **WHEN** a fan opens the concierge panel in Romanian or English
- **THEN** the panel shows the AI suggestion label in that language
- Test: manual check on the deployed preview (concierge panel on a storefront, RO and EN, label visible, phone width)
