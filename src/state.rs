// Copyright (c) Zefchain Labs, Inc.
// SPDX-License-Identifier: Apache-2.0

use std::collections::BTreeSet;

use async_graphql::SimpleObject;
use linera_sdk::{base::AccountOwner, views::{linera_views, MapView, RegisterView, RootView, ViewStorageContext}, DataBlobHash};
use non_fungible::{Nft, TokenId};

/// The application state.
#[derive(RootView, SimpleObject)]
#[view(context = "ViewStorageContext")]
pub struct NonFungibleTokenState {
    // Map from token ID to the NFT data
    pub nfts: MapView<TokenId, Nft>,
    // Map from owners to the set of NFT token IDs they own
    pub owned_token_ids: MapView<AccountOwner, BTreeSet<TokenId>>,
    // chain owned to the set of NFTs for multiple chains
    pub blob_token_ids: MapView<u64, TokenId>,
    // Counter of NFTs minted in this chain, used for hash uniqueness
    pub num_minted_nfts: RegisterView<u64>,
}
