const preloadConfig = {
    fromCache: true,
    cancelWhenRouteChange: false
}

export function getStaticViewAuth (app) {
    const viewAuthorities = []
    app.$router.options.routes.forEach(route => {
        const meta = route.meta || {}
        const auth = meta.auth || {}
        const view = auth.view || ''
        if (view && (typeof view !== 'function')) {
            const [ type ] = view.split('.')
            viewAuthorities.push({
                resource_type: type,
                action: 'read'
            })
        }
    })
    return app.$store.dispatch('auth/getStaticViewAuth', viewAuthorities)
}

export function getClassifications (app) {
    return app.$store.dispatch('objectModelClassify/searchClassificationsObjects', {
        params: app.$injectMetadata(),
        config: {
            ...preloadConfig,
            requestId: 'post_searchClassificationsObjects'
        }
    })
}

export function getBusiness (app) {
    return app.$store.dispatch('objectBiz/searchBusiness', {
        params: {
            'fields': ['bk_biz_id', 'bk_biz_name'],
            'condition': {
                'bk_data_status': {
                    '$ne': 'disabled'
                }
            }
        },
        config: {
            ...preloadConfig,
            requestId: 'post_searchBusiness_$ne_disabled'
        }
    }).then(business => {
        app.$store.commit('objectBiz/setBusiness', business.info)
        return business
    })
}

export function getUserCustom (app) {
    return app.$store.dispatch('userCustom/searchUsercustom', {
        config: {
            ...preloadConfig,
            fromCache: false,
            requestId: 'post_searchUsercustom'
        }
    })
}

export function getUserList (app) {
    return app.$store.dispatch('getUserList').then(list => {
        window.CMDB_USER_LIST = list
        app.$store.commit('setUserList', list)
        return list
    }).catch(e => {
        window.CMDB_USER_LIST = []
    })
}

export default async function (app) {
    await getStaticViewAuth(app)
    await getBusiness(app)
    return Promise.all([
        getClassifications(app),
        getUserCustom(app),
        getUserList(app)
    ])
}
